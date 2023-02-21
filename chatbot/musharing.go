package chatbot

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	api "muvtuberdriver/chatbot/musharing_chatbot/v1"
	"muvtuberdriver/model"
)

type MusharingChatbot struct {
	Server string

	client api.ChatbotServiceClient
	close  chan struct{} // close grpc conn
}

func (m *MusharingChatbot) Chat(textIn *model.TextIn) (*model.TextOut, error) {
	resp, err := m.client.Chat(context.Background(), &api.ChatRequest{
		Prompt: textIn.Content,
	})

	if err != nil {
		return nil, err
	}

	r := model.TextOut{
		Author:   "MusharingChatbot",
		Content:  resp.GetResponse(),
		Priority: textIn.Priority,
	}
	return &r, nil
}

func (m *MusharingChatbot) Close() error {
	close(m.close)
	return nil
}

func NewMusharingChatbot(server string) (Chatbot, error) {
	// conn, err := grpc.Dial(server, grpc.WithInsecure())
	conn, err := grpc.Dial(server, grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		return nil, err
	}

	apiClient := api.NewChatbotServiceClient(conn)

	m := &MusharingChatbot{
		Server: server,
		client: apiClient,
	}

	go func() {
		<-m.close
		_ = conn.Close()
	}()

	return m, nil
}
