package delta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// maxCount limits a retain or delete read from JSON, which keeps sums of
// counts from overflowing an int. No document comes close.
const maxCount = 1 << 30

// MarshalJSON writes d as quill-delta does: {"ops":[...]}.
func (d Delta) MarshalJSON() ([]byte, error) {
	ops := d.Ops
	if ops == nil {
		ops = []Op{}
	}
	return json.Marshal(struct {
		Ops []Op `json:"ops"`
	}{ops})
}

// UnmarshalJSON reads {"ops":[...]}. An empty list becomes nil Ops,
// as the methods of [Delta] return.
func (d *Delta) UnmarshalJSON(b []byte) error {
	var v struct {
		Ops []Op `json:"ops"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if len(v.Ops) == 0 {
		v.Ops = nil
	}
	d.Ops = v.Ops
	return nil
}

// MarshalJSON writes op like quill-delta: {"insert":"text"},
// {"insert":{"image":"src"}}, {"retain":3} or {"delete":3}. An insert or
// retain with formats also gets "attributes".
func (op Op) MarshalJSON() ([]byte, error) {
	var v struct {
		Insert     any        `json:"insert,omitempty"`
		Retain     int        `json:"retain,omitempty"`
		Delete     int        `json:"delete,omitempty"`
		Attributes Attributes `json:"attributes,omitempty"`
	}
	switch op.kind() {
	case kindInsert:
		v.Insert = op.Insert
		if op.IsEmbed() {
			v.Insert = map[string]string{op.Embed.Type: op.Embed.Value}
		}
		v.Attributes = op.Attributes
	case kindRetain:
		v.Retain = op.Retain
		v.Attributes = op.Attributes
	case kindDelete:
		v.Delete = op.Delete
	}
	return json.Marshal(v)
}

// UnmarshalJSON reads 1 operation and rejects what quill-delta never
// produces for text and images: an empty insert, an embed other than 1 key
// with a string value, a count that is not a positive integer, a delete with
// attributes, 0 or several actions.
func (op *Op) UnmarshalJSON(b []byte) error {
	var v struct {
		Insert     json.RawMessage `json:"insert"`
		Retain     json.RawMessage `json:"retain"`
		Delete     json.RawMessage `json:"delete"`
		Attributes Attributes      `json:"attributes"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return err
	}
	actions := 0
	for _, raw := range []json.RawMessage{v.Insert, v.Retain, v.Delete} {
		if raw != nil {
			actions++
		}
	}
	if actions != 1 {
		return errors.New("an operation needs exactly 1 of insert, retain and delete")
	}
	switch {
	case v.Insert != nil:
		ins, err := parseInsert(v.Insert)
		if err != nil {
			return err
		}
		ins.Attributes = v.Attributes
		*op = ins
	case v.Retain != nil:
		n, err := parseCount("retain", v.Retain)
		if err != nil {
			return err
		}
		*op = Op{Retain: n, Attributes: v.Attributes}
	default:
		n, err := parseCount("delete", v.Delete)
		if err != nil {
			return err
		}
		if v.Attributes != nil {
			return errors.New("delete with attributes")
		}
		*op = Op{Delete: n}
	}
	return nil
}

func parseInsert(raw json.RawMessage) (Op, error) {
	if raw[0] == '{' {
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return Op{}, fmt.Errorf("embed: %w", err)
		}
		if len(m) != 1 {
			return Op{}, errors.New("an embed needs exactly 1 key")
		}
		for k, v := range m {
			if k == "" {
				return Op{}, errors.New("embed without a type")
			}
			return Op{Embed: Embed{Type: k, Value: v}}, nil
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return Op{}, err
	}
	if s == "" {
		return Op{}, errors.New("empty insert")
	}
	return Op{Insert: s}, nil
}

func parseCount(action string, raw json.RawMessage) (int, error) {
	n, err := strconv.Atoi(string(raw))
	if err != nil || n <= 0 || n > maxCount {
		return 0, fmt.Errorf("%s is not a positive integer up to %d: %s",
			action, maxCount, raw)
	}
	return n, nil
}

// UnmarshalJSON reads formats with string, number, boolean or null values.
// An empty object becomes nil.
func (a *Attributes) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for k, v := range m {
		switch v.(type) {
		case string, float64, bool, nil:
		default:
			return fmt.Errorf("attribute %q is not a string, number, boolean or null", k)
		}
	}
	*a = Attributes(m).orNil()
	return nil
}
