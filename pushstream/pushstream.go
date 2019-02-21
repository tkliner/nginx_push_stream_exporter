package pushstream

type PushStream struct {
	Channels string `json:"channels"`
	Infos []*Channel `json:"infos"`
}

type Channel struct {
	Channel           string `json:"channel"`
	PublishedMessages string `json:"published_messages"`
	StoredMessages    string `json:"stored_messages"`
	Subscribers       string `json:"subscribers"`
}

func NewPushStream() *PushStream {
	return &PushStream{}
}
