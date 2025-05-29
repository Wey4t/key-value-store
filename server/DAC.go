package server

import (
	"fmt"
	. "types"
)

var Keys = make(map[string]Key)

type get func(k Key) []string
type set func(k *Key, v []string)

func __print_dac__() {
	for k := range Keys {
		fmt.Println("key", Keys[k])
	}
}

var KeyGetter = map[string]get{
	"readers":   func(k Key) []string { return k.Readers },
	"writers":   func(k Key) []string { return k.Writers },
	"copyfroms": func(k Key) []string { return k.Copyfroms },
	"copytos":   func(k Key) []string { return k.Copytos },
	"indirects": func(k Key) []string { return k.Indirects },
}
var KeySetter = map[string]set{
	"readers":   func(k *Key, v []string) { k.Readers = v },
	"writers":   func(k *Key, v []string) { k.Writers = v },
	"copyfroms": func(k *Key, v []string) { k.Copyfroms = v },
	"copytos":   func(k *Key, v []string) { k.Copytos = v },
	"indirects": func(k *Key, v []string) { k.Indirects = v },
}

func isOwner(uid string, key string) bool {
	k, ok := Keys[key]
	if !ok || k.Owner != uid {
		return false
	}
	return true
}
func DeleteKey(uid string, key string) bool {
	k, ok := Keys[key]
	if !ok || k.Owner != uid {
		return false
	}
	delete(Keys, key)
	return true
}

func Create(uid string, key string, metadata map[string][]string) bool {
	_, ok := Keys[key]
	if ok {
		return false
	}

	for k, v := range metadata {
		if v == nil {
			metadata[k] = []string{}
		}
	}
	Keys[key] = Key{
		Readers:   metadata["readers"],
		Writers:   metadata["writers"],
		Copyfroms: metadata["copyfroms"],
		Copytos:   metadata["copytos"],
		Indirects: metadata["indirects"],
		Owner:     uid,
	}
	return true
}
func Modacl(uid string, key string, metadata map[string][]string) bool {

	old, ok := Keys[key]
	if !ok || old.Owner != uid {
		return false
	}
	for attr, v := range metadata {
		setter, ok := KeySetter[attr]
		if v != nil && ok {
			setter(&old, v)
		}
	}
	Keys[key] = old
	return true
}
func Revacl(uid string, key string) map[string][]string {
	old, ok := Keys[key]
	if !ok || old.Owner != uid {
		return nil
	}
	result := make(map[string][]string)
	result["readers"] = old.Readers
	result["writers"] = old.Writers
	result["copyfroms"] = old.Copyfroms
	result["copytos"] = old.Copytos
	result["indirects"] = old.Indirects
	result["R"] = R(key)
	result["W"] = W(key)
	result["Csrc"] = Csrc(key)
	result["Cdst"] = Cdst(key)
	return result
}

func R(key string) []string {
	return bfs(key, "readers")
}
func W(key string) []string {
	return bfs(key, "writers")

}
func Csrc(key string) []string {
	return bfs(key, "copyfroms")
}
func Cdst(key string) []string {
	return bfs(key, "copytos")
}

func bfs(key string, attr string) []string {
	visited := make(map[string]bool)
	principal := make(map[string]bool)
	queue := []string{key}
	for len(queue) > 0 {
		curkey := queue[0]
		queue = queue[1:]
		k_obj, ok := Keys[curkey]
		if !visited[curkey] && ok {
			visited[curkey] = true
			attrGetter := KeyGetter[attr]
			for _, p := range attrGetter(k_obj) {
				if !principal[p] {
					principal[p] = true
				}
			}
			indirectGetter := KeyGetter["indirects"]
			for _, nextKey := range indirectGetter(k_obj) {
				queue = append(queue, nextKey)
			}
		}
	}
	result := make([]string, 0, len(principal))
	for p := range principal {
		result = append(result, p)
	}
	return result
}
