package console

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQSession 은 전역 RabbitMQ 세션 인스턴스
var RabbitMQSession *rabbitMQSession
var rmqOnce sync.Once

// rabbitMQSession 은 RabbitMQ 연결과 채널을 관리합니다
type rabbitMQSession struct {
	Conn      *amqp.Connection
	Channels  map[string]*amqp.Channel
	Reconnect chan bool
	mu        sync.RWMutex
}

// initRabbitMQSession 은 RabbitMQ 세션을 초기화합니다 (init.go에서 호출)
func initRabbitMQSession() error {
	var initErr error

	rmqOnce.Do(func() {
		connStr := fmt.Sprintf("amqp://%s:%s@%s:%s/",
			Env.RABBITMQ_USER, Env.RABBITMQ_PASS,
			Env.RABBITMQ_HOST, Env.RABBITMQ_PORT,
		)

		conn, err := amqp.Dial(connStr)
		if err != nil {
			initErr = fmt.Errorf("RabbitMQ 연결 실패: %w", err)
			return
		}

		RabbitMQSession = &rabbitMQSession{
			Conn:      conn,
			Channels:  make(map[string]*amqp.Channel),
			Reconnect: make(chan bool, 1),
		}

		fmt.Printf("%s [RabbitMQ] 세션 초기화 성공\n", GenerateTimestampedString())
	})

	return initErr
}

// AddChannelAndQueue 는 채널을 생성하고 큐를 선언합니다
func (s *rabbitMQSession) AddChannelAndQueue(queueName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch, err := s.Conn.Channel()
	if err != nil {
		return fmt.Errorf("채널 열기 실패: %w", err)
	}

	_, err = ch.QueueDeclare(
		queueName,
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		ch.Close()
		return fmt.Errorf("큐 선언 실패: %w", err)
	}

	s.Channels[queueName] = ch
	fmt.Printf("%s [RabbitMQ] 채널 및 큐 생성: %s\n", GenerateTimestampedString(), queueName)
	return nil
}

// RemoveChannelAndQueue 는 채널과 큐를 제거합니다
func (s *rabbitMQSession) RemoveChannelAndQueue(queueName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch, exists := s.Channels[queueName]
	if !exists {
		return fmt.Errorf("queue %s not found", queueName)
	}
	err := ch.Close()
	if err != nil {
		return err
	}
	delete(s.Channels, queueName)
	return nil
}

// MonitorConnection 은 연결 종료를 감지합니다
func (s *rabbitMQSession) MonitorConnection() {
	for {
		notifyClose := make(chan *amqp.Error, 1)
		s.Conn.NotifyClose(notifyClose)

		err := <-notifyClose
		if err != nil {
			fmt.Printf("%s [RabbitMQ] 연결 종료됨: %v\n", GenerateTimestampedString(), err)

			// Non-blocking send로 데드락 방지
			select {
			case s.Reconnect <- true:
				fmt.Printf("%s [RabbitMQ] 재연결 신호 전송\n", GenerateTimestampedString())
			default:
				fmt.Printf("%s [RabbitMQ] 재연결이 이미 진행 중\n", GenerateTimestampedString())
			}
		} else {
			fmt.Printf("%s [RabbitMQ] 연결이 정상적으로 종료됨\n", GenerateTimestampedString())
			return
		}
	}
}

// HandleReconnection 은 재연결을 처리합니다
func (s *rabbitMQSession) HandleReconnection() {
	for range s.Reconnect {
		fmt.Printf("%s [RabbitMQ] 재연결 시도 중...\n", GenerateTimestampedString())

		connStr := fmt.Sprintf("amqp://%s:%s@%s:%s/",
			Env.RABBITMQ_USER, Env.RABBITMQ_PASS,
			Env.RABBITMQ_HOST, Env.RABBITMQ_PORT,
		)

		for {
			conn, err := amqp.Dial(connStr)
			if err == nil {
				if s.Conn != nil {
					s.Conn.Close()
				}

				s.Conn = conn
				fmt.Printf("%s [RabbitMQ] 재연결 성공\n", GenerateTimestampedString())

				// 기존 채널들을 다시 생성
				if err := s.recreateChannels(); err != nil {
					fmt.Printf("%s [RabbitMQ] 채널 재생성 실패: %v\n", GenerateTimestampedString(), err)
					conn.Close()
					continue
				}

				// 새 연결에 대한 모니터링 시작
				go s.MonitorConnection()
				break
			}

			fmt.Printf("%s [RabbitMQ] 재연결 실패: %v. 5초 후 재시도...\n", GenerateTimestampedString(), err)
			time.Sleep(5 * time.Second)
		}
	}
}

// recreateChannels 는 재연결 후 기존 채널들을 다시 생성합니다
func (s *rabbitMQSession) recreateChannels() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 기존 큐 이름들을 저장
	queueNames := make([]string, 0, len(s.Channels))
	for name := range s.Channels {
		queueNames = append(queueNames, name)
	}

	// 기존 채널 맵 초기화
	s.Channels = make(map[string]*amqp.Channel)

	// 각 큐를 다시 생성
	for _, queueName := range queueNames {
		ch, err := s.Conn.Channel()
		if err != nil {
			return fmt.Errorf("채널 재생성 실패 %s: %w", queueName, err)
		}

		_, err = ch.QueueDeclare(queueName, false, false, false, false, nil)
		if err != nil {
			ch.Close()
			return fmt.Errorf("큐 재선언 실패 %s: %w", queueName, err)
		}

		s.Channels[queueName] = ch
		fmt.Printf("%s [RabbitMQ] 채널 재생성: %s\n", GenerateTimestampedString(), queueName)
	}

	return nil
}

// Send 는 메시지를 큐에 전송합니다
func (s *rabbitMQSession) Send(queueName string, data []byte) error {
	s.mu.RLock()
	channel, exists := s.Channels[queueName]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("큐 %s의 채널을 찾을 수 없음", queueName)
	}

	if channel.IsClosed() {
		return fmt.Errorf("큐 %s의 채널이 닫혀 있음", queueName)
	}

	if s.Conn != nil && s.Conn.IsClosed() {
		return fmt.Errorf("RabbitMQ 연결이 닫혀 있음")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := channel.PublishWithContext(ctx,
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        data,
		},
	)

	if err != nil {
		return fmt.Errorf("메시지 전송 실패: %w", err)
	}

	return nil
}

// Receive 는 큐에서 메시지를 수신합니다
func (s *rabbitMQSession) Receive(queueName string, msgChan chan<- []byte) error {
	s.mu.RLock()
	channel, exists := s.Channels[queueName]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("큐 %s의 채널을 찾을 수 없음", queueName)
	}

	msgs, err := channel.Consume(
		queueName,
		"",    // consumer
		true,  // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("컨슈머 등록 실패: %w", err)
	}

	go func() {
		for d := range msgs {
			msgChan <- d.Body
		}
	}()

	return nil
}

// AddConsumerChannel 은 외부 발행자가 만든 기존 큐를 소비하기 위한 채널을 연다 (2026-07-09 listen 모드).
// QueueDeclarePassive로 존재 확인만 하므로 큐 속성(durable 등)과 무관하게 붙을 수 있다.
// 주의: 재연결 시 recreateChannels가 능동 declare를 시도하므로, durable 큐 소비 중 재연결이 나면
// 속성 충돌로 실패할 수 있다 — listen 모드는 그 경우 재기동이 필요하다.
func (s *rabbitMQSession) AddConsumerChannel(queueName string) error {
	ch, err := s.Conn.Channel()
	if err != nil {
		return fmt.Errorf("채널 생성 실패: %w", err)
	}
	if _, err := ch.QueueDeclarePassive(queueName, false, false, false, false, nil); err != nil {
		// durable 큐 가능성 — 새 채널로 durable=true 재시도 (passive는 존재 확인이 목적)
		ch2, err2 := s.Conn.Channel()
		if err2 != nil {
			return fmt.Errorf("채널 재생성 실패: %w", err2)
		}
		if _, err3 := ch2.QueueDeclarePassive(queueName, true, false, false, false, nil); err3 != nil {
			return fmt.Errorf("큐 %s 존재 확인 실패: %v / %v", queueName, err, err3)
		}
		ch = ch2
	}
	s.mu.Lock()
	s.Channels[queueName] = ch
	s.mu.Unlock()
	return nil
}

// NotifyConnClose 는 RabbitMQ 연결 종료 이벤트 채널을 반환한다 (2026-07-09, listen 상시 가동용).
// 상시 데몬은 이 채널 수신 시 즉시 비정상 종료해 systemd 등의 재기동에 맡기는 것이 안전하다
// (내부 재연결 로직은 소비자 채널까지는 복구하지 못한다).
func (s *rabbitMQSession) NotifyConnClose() <-chan *amqp.Error {
	return s.Conn.NotifyClose(make(chan *amqp.Error, 1))
}

// SendStr 는 문자열 메시지를 큐에 전송하는 간편 함수 (자동 로그 포함)
func SendStr(queueName string, message string) error {
	msgBytes := []byte(message)
	err := RabbitMQSession.Send(queueName, msgBytes)

	if err != nil {
		LogError("메시지 전송 실패 [%s]: %v", queueName, err)
		return err
	}

	Log("메시지 전송 완료 [%s]: %s", queueName, message)
	return nil
}

// SendJson 는 구조체를 JSON으로 변환하여 큐에 전송하는 간편 함수 (자동 로그 포함)
func SendJson(queueName string, data interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		LogError("JSON 마샬링 실패 [%s]: %v", queueName, err)
		return err
	}

	err = RabbitMQSession.Send(queueName, jsonBytes)
	if err != nil {
		LogError("메시지 전송 실패 [%s]: %v", queueName, err)
		return err
	}

	Log("JSON 메시지 전송 완료 [%s]: %s", queueName, string(jsonBytes))
	return nil
}
