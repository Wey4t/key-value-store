package types

type Blindentry struct {
	Uid        string `json:"uid"`
	Tod        []byte `json:"tod"`
	PublicKey  []byte `json:"publicKey"`
	SessionKey []byte `json:"sessionKey"`
}
