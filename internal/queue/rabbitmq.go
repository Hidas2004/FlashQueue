package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// 1. RABBITMQ MESSAGE
var _ Message = (*rabbitMQMessage)(nil)

type rabbitMQMessage struct {
	// amqp.delivery là dữ lệu mà RabbitMQ gửi đến cho consumer, nó chứa thông tin về message và các phương thức để ack/nack message
	delivery amqp.Delivery
}

func (m *rabbitMQMessage) Body() []byte {
	// Trả về nội dung của message dưới dạng byte {"order_id": 123}
	return m.delivery.Body
}

func (m *rabbitMQMessage) Ack() error {
	//ACKxác nhận đã xử lý thnahf công message,rabbitMQ sẽ xóa message khỏi hàng đợi
	return m.delivery.Ack(false)
}

func (m *rabbitMQMessage) Nack(requeue bool) error {
	//NACKxác nhận đã xử lý message thất bại, nếu requeue là true thì message sẽ được đưa trở lại hàng đợi để xử lý lại, nếu false thì message sẽ bị loại bỏ
	return m.delivery.Nack(false, requeue)
}

// 2. RABBITMQ CONNECTION MANAGER

type RabbitMQ struct {
	conn      *amqp.Connection //thứ sẽ được tái sử dụng để tạo channel mới cho publisher và consumer, giúp tiết kiệm tài nguyên và cải thiện hiệu suất
	QueueName string
	DLQName   string
}

func NewRabbitMQFromURL(url, queueName, dlqName string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	// hàm này sẽ được gọi khi hàm NewRabbitMQFromURL kết thúc thành công hay thất bại, nó sẽ đóng channel để giải phóng tài nguyên
	defer ch.Close()
	// Khai báo DLQ
	_, err = ch.QueueDeclare(dlqName, true, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("declare DLQ: %w", err)
	}
	// Khai báo Main Queue
	// Khai báo Main Queue
	_, err = ch.QueueDeclare(
		queueName, true, false, false, false,
		amqp.Table{
			"x-dead-letter-exchange":    "",              //nơi message sẽ bị chuyển tới nếu:Bị Nack(false, false),Bị reject,Hết TTL,Queue đầy
			"x-dead-letter-routing-key": dlqName,         //Dead Letter Exchange sẽ sử dụng routing key này để định tuyến message đến DLQ
			"x-message-ttl":             int32(86400000), // 24h
		},
	)
	if err != nil {
		return nil, fmt.Errorf("declare main queue: %w", err)
	}
	return &RabbitMQ{
		conn:      conn,
		QueueName: queueName,
		DLQName:   dlqName,
	}, nil
}

// 3. PUBLISHER IMPLEMENTATION
type RabbitPublisher struct {
	rmq *RabbitMQ
	ch  *amqp.Channel
}

func NewPublisher(rmq *RabbitMQ) (*RabbitPublisher, error) {
	ch, err := rmq.conn.Channel() //gọi channel từ kết nối có sẵn rmq.conn
	if err != nil {
		return nil, err
	}
	return &RabbitPublisher{
		rmq: rmq,
		ch:  ch,
	}, nil
}

func (p *RabbitPublisher) Publish(ctx context.Context, msg interface{}) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return p.ch.PublishWithContext(ctx, "", p.rmq.QueueName, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         body,
	})

}

func (p *RabbitPublisher) PublishToDLQ(ctx context.Context, msg interface{}) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return p.ch.PublishWithContext(ctx, "", p.rmq.DLQName, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         body,
	})
}
func (p *RabbitPublisher) Close() error {
	return p.ch.Close()
}

// 4. CONSUMER IMPLEMENTATION

type RabbitConsumer struct {
	rmq *RabbitMQ
	ch  *amqp.Channel
}

func NewConSumer(rmq *RabbitMQ) (*RabbitConsumer, error) {
	ch, err := rmq.conn.Channel()
	if err != nil {
		return nil, err
	}
	// Qos: Báo RabbitMQ chỉ đẩy 1 message cho worker này xong chờ Ack rồi mới đẩy tiếp
	if err := ch.Qos(1, 0, false); err != nil {
		return nil, err
	}
	return &RabbitConsumer{
		rmq: rmq,
		ch:  ch,
	}, nil
}

// hàm tiêu thụ message
func (c *RabbitConsumer) Consume() (<-chan Message, error) {
	deliveries, err := c.ch.Consume(c.rmq.QueueName, "", false, false, false, false, nil)
	if err != nil {
		return nil, err
	}
	// Wrapper channel: Chuyển đổi từ amqp.Delivery sang interface Message của mình
	msgCh := make(chan Message)
	go func() {
		// Goroutine này chạy ngầm để map data
		for d := range deliveries { //lấy d từ channel deliveries, mỗi d là một amqp.Delivery
			msgCh <- &rabbitMQMessage{delivery: d} //gửi một rabbitMQMessage mới vào msgCh, trong đó delivery được gán bằng d
		}
		close(msgCh)
	}()
	return msgCh, nil
}

func (c *RabbitConsumer) Close() error {
	return c.ch.Close()
}
