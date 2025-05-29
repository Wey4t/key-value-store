package types

type Message struct {
	Uid       string `json:"uid"`
	Status    string `json:"status"`
	PublicKey []byte `json:"publickey"`
	Time      []byte `json:"time"`
	Values    []byte `json:"values"`
	Pass      string `json:"pass"`
	Old_pass  string `json:"old_pass"`
	New_pass  string `json:"new_pass"`
}
