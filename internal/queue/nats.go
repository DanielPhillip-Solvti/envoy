package queue

import (
	"encoding/json"

	"github.com/nats-io/nats.go"
)

type Bus struct {
	nc *nats.Conn
}

func Connect(url, nkey string) (*Bus, error) {
	opts := []nats.Option{nats.Name("envoy")}
	if nkey != "" {
		opts = append(opts, nats.UserCredentials(nkey))
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, err
	}
	return &Bus{nc: nc}, nil
}

func (b *Bus) Close() {
	b.nc.Close()
}

func (b *Bus) PublishJSON(subject string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return b.nc.Publish(subject, data)
}

func (b *Bus) SubscribeJSON(subject string, handler func([]byte)) (*nats.Subscription, error) {
	return b.nc.Subscribe(subject, func(msg *nats.Msg) {
		handler(msg.Data)
	})
}
