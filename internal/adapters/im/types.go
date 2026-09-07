package im

import "context"

type Backend interface {
	SendMessage(ctx context.Context, req SendMessageRequest) (*Message, error)
}

type SendMessageRequest struct {
	Target   string
	Body     string
	Mentions []string
}

type Message struct {
	ID       string
	RawJSON  string
	Provider string
}
