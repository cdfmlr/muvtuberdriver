package chatbot

type musharingChatbot struct {
	*SessionClientsPool
}

func NewMusharingChatbot(server string) (Chatbot, error) {
	scp, err := NewSessionClientsPool(server, NoChatbotConfig{})
	if err != nil {
		return nil, err
	}

	scp.Name = "MusharingChatbot"
	scp.Verbose = true

	return &musharingChatbot{
		SessionClientsPool: scp,
	}, nil
}
