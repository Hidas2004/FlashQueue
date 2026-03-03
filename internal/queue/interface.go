package queue

import "context"

type Message interface {
	Body() []byte
	Ack() error
	Nack(requeue bool) error
}

// Consumer (Người tiêu thụ) định nghĩa hành động lấy message ra
type Consumer interface {
	Consume() (<-chan Message, error)
	Close() error
}

// Publisher (Người xuất bản) định nghĩa hành động gửi message đi
type Publisher interface {
	Publish(ctx context.Context, msg interface{}) error
	PublishToDLQ(ctx context.Context, msg interface{}) error
	Close() error
}
