package pushstream

type PushStream struct {
	Channels int64 `json:"channels"`
	Infos []*Channel `json:"infos"`
}

type Channel struct {
	Channel           string `json:"channel"`
	PublishedMessages int64 `json:"published_messages"`
	StoredMessages    int64 `json:"stored_messages"`
	Subscribers       int64 `json:"subscribers"`
}

func NewPushStream() *PushStream {
	return &PushStream{}
}
