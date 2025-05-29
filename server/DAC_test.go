package server

import (
	"sort"
	"testing"
	. "types"
)

func TestCreate(t *testing.T) {
	Keys = make(map[string]Key)
	var metadata = map[string][]string{
		"readers": []string{"fbs"}}
	ok := Create("fbs", "gs", metadata)
	if !ok {

		t.Errorf("fail")
	}
}

func TestRevacl(t *testing.T) {
	Keys = make(map[string]Key)
	var metadata = map[string][]string{
		"readers": []string{"fbs"}}
	Create("fbs", "gs", metadata)
	wrong_uid := Revacl("fbss", "gs")
	wrong_key := Revacl("fbs", "gss")
	if wrong_key != nil || wrong_uid != nil {
		t.Errorf("fail")

	}
	correct := Revacl("fbs", "gs")

	if correct["readers"][0] != "fbs" {
		t.Errorf("fail")

	}
}

func compare(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	} else {
		sort.Slice(a, func(i, j int) bool {
			return a[i] < a[j]
		})
		sort.Slice(b, func(i, j int) bool {
			return b[i] < b[j]
		})
		for i := 0; i < len(a); i += 1 {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
}
func TestModacl(t *testing.T) {
	Keys = make(map[string]Key)
	var metadata = map[string][]string{
		"readers":   []string{"fbs", "gs", "kz", "a", "b"},
		"writers":   []string{"a", "b"},
		"copyfroms": []string{"fbs", "gs", "kz", "a", "b"},
		"copytos":   []string{"a", "b"},
		"indirects": []string{"B"},
	}
	Create("fbs", "A", metadata)

	metadata = map[string][]string{
		"readers":   []string{"fbs", "gs", "kz"},
		"writers":   []string{"fbs", "gs", "kz"},
		"copyfroms": []string{"fbs", "gs", "kz"},
		"copytos":   []string{"fbs", "gs", "kz"},
		"indirects": []string{"A"},
	}
	Create("std1", "B", metadata)

	correct := Revacl("fbs", "A")
	if correct["readers"][0] != "fbs" {
		t.Errorf("fail")
	}

	wrong_uid := Modacl("fbss", "A", metadata)
	wrong_key := Modacl("fbs", "B", metadata)
	if wrong_key || wrong_uid {
		t.Errorf("fail")
	}
	metadata = map[string][]string{
		"readers": []string{"fbs"},
		"writers": []string{"fbs", "kz"},
	}
	ok := Modacl("fbs", "A", metadata)
	if !ok {
		t.Errorf("fail")
	}

	empty := Revacl("fbs", "gs")
	if empty != nil {
		t.Errorf("fail")
	}
	B := Revacl("std1", "B")
	if !compare(B["readers"], []string{"fbs", "kz", "gs"}) {
		t.Errorf("fail")
	}
}
func TestDelete(t *testing.T) {
	Keys = make(map[string]Key)
	var metadata = map[string][]string{
		"readers":   []string{"fbs", "gs", "kz", "a", "b"},
		"writers":   []string{"a", "b"},
		"copyfroms": []string{"fbs", "gs", "kz", "a", "b"},
		"copytos":   []string{"a", "b"},
		"indirects": []string{"B"},
	}
	Create("fbs", "A", metadata)

	result := DeleteKey("fbb", "A")

	if result {
		t.Errorf("fail")
	}
	rev := Revacl("fbs", "A")
	revD := Revacl("fbs", "A")

	result = DeleteKey("fbs", "A")
	if !result && rev == nil && revD != nil {
		t.Errorf("fail")
	}

}
