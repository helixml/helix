package types

type UIAtResponse struct {
	Data []UIAtData `json:"data"`
}

type UIAtData struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
