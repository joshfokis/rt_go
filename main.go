// Language: go
// Path: main_test.go
// main package for request tracker
package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

//	"flag"

//	"log"
//	"os"

// RTer interface
type RTer interface {
	GetTicket(id int) (*Ticket, error)
	GetTicketHistory(id int) ([]TicketHistory, error)
	GetTicketTransactions(id int) ([]TicketTransaction, error)
	GetTicketLinks(id int) ([]TicketLink, error)
}

// Ticket struct for RT tickets
type Ticket struct {
	ID              string `rt:"id"`
	Queue           string
	Owner           string
	Creator         string
	Subject         string
	Status          string
	Priority        int
	InitialPriority int
	FinalPriority   int
	Requestors      string
	Cc              string
	AdminCc         string
	Created         time.Time
	Starts          time.Time
	Started         time.Time
	Due             time.Time
	Resolved        time.Time
	Told            time.Time
	LastUpdated     time.Time
	TimeEstimated   int
	TimeWorked      int
	TimeLeft        int
	customFields    map[string]string
}

// TicketComment struct for RT ticket comments
type TicketComment struct {
	ID        int
	Creator   string
	Created   time.Time
	Content   string
	IsPrivate bool
}

// TicketCustomFieldHistory struct for RT ticket custom field history
type TicketCustomFieldHistory struct {
	Field string
	Old   string
	New   string
}

// TicketCustomFieldValuesHistory struct
type TicketCustomFieldValuesHistory struct {
	OldValue string
	NewValue string
}

// TicketLink struct
type TicketLink struct {
	Type string
	ID   int
}

// TicketCustomField struct
type TicketCustomField struct {
	ID    int
	Name  string
	Value string
}

// TicketAttachment struct
type TicketAttachment struct {
	ID          int
	Filename    string
	Content     string
	MimeType    string
	Creator     string
	Created     time.Time
	LastUpdated time.Time
}

// Attachment Struct
type Attachment struct {
	ID          int
	Filename    string
	Description string
	ContentType string
	Content     string
}

// TicketTransaction struct for RT ticket transactions
type TicketTransaction struct {
	ID          int
	Type        string
	Field       string
	OldValue    string
	NewValue    string
	Data        string
	Object      string
	Creator     string
	Created     time.Time
	Attachments []Attachment
}

// TicketHistory struct for RT ticket history
type TicketHistory struct {
	OldValue string
	NewValue string
	Field    string
	Creator  string
	Created  time.Time
}

// RT struct
type RT struct {
	URL      string
	Username string
	Password string
	Client   *http.Client
}

// NewRT function
func NewRT(url string, username string, password string) *RT {
	return &RT{
		URL:      url,
		Username: username,
		Password: password,
		Client:   &http.Client{},
	}
}

// UnmarshalShort function
func UnmarshalShort(dataStr string, v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("cannot unmarshal into type %v, not a pointer", reflect.TypeOf(v))
	}
	el := rv.Elem()
	if el.Kind() != reflect.Struct {
		return fmt.Errorf("cannot unmarshal into non-struct pointer")
	}
	elType := el.Type()

	// map from names to values that we should assign
	values := make(map[string]reflect.Value)

	for i := 0; i < el.NumField(); i++ {
		structField := elType.Field(i)
		name := structField.Tag.Get("rt")
		if name == "" {
			name = structField.Name
		}
		values[name] = el.Field(i)
	}

	// convert data string to string key-value map
	data := make(map[string]string)
	var lastKey string
	for _, line := range strings.Split(dataStr, "\n") {
		if len(line) == 0 {
			continue
		} else if line[0] == ' ' && lastKey != "" {
			// multiline values are prefixed with a space
			data[lastKey] += "\n" + strings.TrimSpace(line)
		}

		parts := strings.SplitN(line, ":", 2)

		if len(parts) != 2 {
			return fmt.Errorf("data has line without colon: %s", line)
		}

		lastKey = parts[0]

		if len(parts[1]) > 0 && parts[1][0] == ' ' {
			data[lastKey] = parts[1][1:]
		} else {
			data[lastKey] = parts[1]
		}
	}

	// convert values to struct field
	for key, value := range data {
		if value == "" || value == "Not set" {
			continue
		}

		rv, ok := values[key]
		if !ok {
			continue
		}
		rvType := rv.Type()

		if rvType.PkgPath() == "time" && rvType.Name() == "Time" {
			t, err := time.Parse("Mon Jan 2 15:04:05 2006", value)
			if err == nil {
				rv.Set(reflect.ValueOf(t))
			} else {
				return fmt.Errorf("failed to decode %s as time: %v", value, err)
			}
		} else if rv.Kind() == reflect.String {
			rv.SetString(value)
		} else if rv.Kind() == reflect.Int {
			x, err := strconv.Atoi(value)
			if err == nil {
				rv.SetInt(int64(x))
			} else {
				return fmt.Errorf("failed to decode %s as int: %v", value, err)
			}
		}
	}

	return nil
}

// request function
func (rt *RT) request(path string, params url.Values, v interface{}, isList bool) (int, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("user", rt.Username)
	params.Set("pass", rt.Password)

	resp, err := rt.Client.Get(rt.URL + path + "?" + params.Encode())
	if err != nil {
		return 0, fmt.Errorf("error performing HTTP request: %v", err)
	}

	if resp.StatusCode == 401 {
		return 0, fmt.Errorf("server refused the provided user credentials")
	} else if resp.StatusCode != 200 {
		return 0, fmt.Errorf("server returned status code %d", resp.StatusCode)
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("error reading HTTP response: %v", err)
	}

	responseString := string(bytes)
	responseParts := strings.SplitN(responseString, "\n", 3)
	if len(responseParts) != 3 {
		return 0, fmt.Errorf("invalid response from server: %s", responseString)
	}

	if !strings.Contains(responseParts[0], "RT") {
		return 0, fmt.Errorf("invalid response from server: %s", responseString)
	}

	if v != nil {
		if isList {
			err = parseList(responseParts[2], v)
		} else {
			err = parseSingle(responseParts[2], v)
		}
		if err != nil {
			return 0, fmt.Errorf("error parsing response: %v", err)
		}
	}

	return resp.StatusCode, nil
}

// parseList function
func parseList(dataStr string, v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("cannot unmarshal into type %v, not a pointer", reflect.TypeOf(v))
	}
	el := rv.Elem()
	if el.Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal into non-slice pointer")
	}
	elType := el.Type().Elem()

	// convert data string to string key-value map
	data := make(map[int]map[string]string)
	var lastKey string
	var lastID int
	for _, line := range strings.Split(dataStr, "\n") {
		if len(line) == 0 {
			continue
		} else if line[0] == ' ' && lastKey != "" {
			// multiline values are prefixed with a space
			data[lastID][lastKey] += "\n" + strings.TrimSpace(line)
		}

		parts := strings.SplitN(line, ":", 2)

		if len(parts) != 2 {
			return fmt.Errorf("data has line without colon: %s", line)
		}

		lastKey = parts[0]

		if len(parts[1]) > 0 && parts[1][0] == ' ' {
			data[lastID][lastKey] = parts[1][1:]
		} else {
			data[lastID][lastKey] = parts[1]
		}

		if lastKey == "id" {
			lastID, _ = strconv.Atoi(data[lastID][lastKey])
		}
	}

	// convert values to struct field
	for _, data := range data {
		newEl := reflect.New(elType)
		for key, value := range data {
			if value == "" || value == "Not set" {
				continue
			}

			rv, ok := newEl.Elem().Type().FieldByNameFunc(func(name string) bool {
				return strings.EqualFold(name, key)
			})
			if !ok {
				continue
			}
			rvType := rv.Type

			if rvType.PkgPath() == "time" && rvType.Name() == "Time" {
				t, err := time.Parse("Mon Jan 2 15:04:05 2006", value)
				if err == nil {
					newEl.Elem().FieldByName(rv.Name).Set(reflect.ValueOf(t))
				} else {
					return fmt.Errorf("failed to decode %s as time: %v", value, err)
				}
			} else if rvType.Kind() == reflect.String {
				newEl.Elem().FieldByName(rv.Name).SetString(value)
			} else if rvType.Kind() == reflect.Int {
				x, err := strconv.Atoi(value)
				if err == nil {
					newEl.Elem().FieldByName(rv.Name).SetInt(int64(x))
				} else {
					return fmt.Errorf("failed to decode %s as int: %v", value, err)
				}
			}
		}
		el.Set(reflect.Append(el, newEl.Elem()))
	}

	return nil
}

// parseSingle function
func parseSingle(dataStr string, v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("cannot unmarshal into type %v, not a pointer", reflect.TypeOf(v))
	}
	el := rv.Elem()
	if el.Kind() != reflect.Struct {
		return fmt.Errorf("cannot unmarshal into non-struct pointer")
	}

	var lastKey string

	// convert data string to string key-value map
	data := make(map[string]string)
	for _, line := range strings.Split(dataStr, "\n") {
		if len(line) == 0 {
			continue
		} else if line[0] == ' ' && lastKey != "" {
			// multiline values are prefixed with a space
			data[lastKey] += "\n" + strings.TrimSpace(line)
		}

		parts := strings.SplitN(line, ":", 2)

		if len(parts) != 2 {
			return fmt.Errorf("data has line without colon: %s", line)
		}

		lastKey = parts[0]

		if len(parts[1]) > 0 && parts[1][0] == ' ' {
			data[lastKey] = parts[1][1:]
		} else {
			data[lastKey] = parts[1]
		}
	}

	// convert values to struct field
	for key, value := range data {
		if value == "" || value == "Not set" {
			continue
		}

		rv, ok := el.Type().FieldByNameFunc(func(name string) bool {
			return strings.EqualFold(name, key)
		})
		if !ok {
			continue
		}
		rvType := rv.Type

		if rvType.PkgPath() == "time" && rvType.Name() == "Time" {
			t, err := time.Parse("Mon Jan 2 15:04:05 2006", value)
			if err == nil {
				el.FieldByName(rv.Name).Set(reflect.ValueOf(t))
			} else {
				return fmt.Errorf("failed to decode %s as time: %v", value, err)
			}
		} else if rvType.Kind() == reflect.String {
			el.FieldByName(rv.Name).SetString(value)
		} else if rvType.Kind() == reflect.Int {
			x, err := strconv.Atoi(value)
			if err == nil {
				el.FieldByName(rv.Name).SetInt(int64(x))
			} else {
				return fmt.Errorf("failed to decode %s as int: %v", value, err)
			}
		}
	}

	return nil
}

// GetTicket Function
func (rt *RT) GetTicket(id int) (*Ticket, error) {
	ticket := &Ticket{}
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/show", nil, ticket, false)
	return ticket, err
}

// GetTicketHistory Function
func (rt *RT) GetTicketHistory(id int) ([]TicketHistory, error) {
	var history []TicketHistory
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/history", nil, &history, true)
	return history, err
}

// GetTicketTransactions Function
func (rt *RT) GetTicketTransactions(id int) ([]TicketTransaction, error) {
	var transactions []TicketTransaction
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/transactions", nil, &transactions, true)
	return transactions, err
}

// GetTicketLinks Function
func (rt *RT) GetTicketLinks(id int) ([]TicketLink, error) {
	var links []TicketLink
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/links", nil, &links, true)
	return links, err
}

// GetTicketAttachments Function
func (rt *RT) GetTicketAttachments(id int) ([]TicketAttachment, error) {
	var attachments []TicketAttachment
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/attachments", nil, &attachments, true)
	return attachments, err
}

// GetTicketAttachment Function
func (rt *RT) GetTicketAttachment(id int, filename string) ([]byte, error) {
	resp, err := rt.Client.Get(rt.URL + "ticket/" + strconv.Itoa(id) + "/attachments/" + filename)
	if err != nil {
		return nil, fmt.Errorf("error performing HTTP request: %v", err)
	}

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("server refused the provided user credentials")
	} else if resp.StatusCode != 200 {
		return nil, fmt.Errorf("server returned status code %d", resp.StatusCode)
	}

	return ioutil.ReadAll(resp.Body)
}

// GetTicketComments Function
func (rt *RT) GetTicketComments(id int) ([]TicketComment, error) {
	var comments []TicketComment
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/comments", nil, &comments, true)
	return comments, err
}

// GetTicketCustomFields Function
func (rt *RT) GetTicketCustomFields(id int) ([]TicketCustomField, error) {
	var fields []TicketCustomField
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/custom_fields", nil, &fields, true)
	return fields, err
}

// GetTicketCustomField Function
func (rt *RT) GetTicketCustomField(id int, name string) (string, error) {
	var value string
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/custom_fields/"+name, nil, &value, false)
	return value, err
}

// GetTicketCustomFieldValues Function
func (rt *RT) GetTicketCustomFieldValues(id int, name string) ([]string, error) {
	var values []string
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/custom_fields/"+name+"/values", nil, &values, true)
	return values, err
}

// GetTicketCustomFieldHistory Function
func (rt *RT) GetTicketCustomFieldHistory(id int, name string) ([]TicketCustomFieldHistory, error) {
	var history []TicketCustomFieldHistory
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/custom_fields/"+name+"/history", nil, &history, true)
	return history, err
}

// GetTicketCustomFieldValuesHistory Function
func (rt *RT) GetTicketCustomFieldValuesHistory(id int, name string) ([]TicketCustomFieldValuesHistory, error) {
	var history []TicketCustomFieldValuesHistory
	_, err := rt.request("ticket/"+strconv.Itoa(id)+"/custom_fields/"+name+"/values/history", nil, &history, true)
	return history, err
}

func main() {
	rt := NewRT("https://rt.example.com/REST/1.0/", "user", "password")

	ticket, err := rt.GetTicket(1)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println(ticket)
}
