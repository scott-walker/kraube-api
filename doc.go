// Package kraube — легковесный Go-шлюз для доступа к Anthropic Messages API
// через OAuth подписку (Claude Pro/Max/Team).
//
// Kraube API реплицирует HTTP-протокол Claude Code CLI — billing header,
// metadata.user_id, model-specific beta headers, Chrome TLS fingerprint —
// предоставляя полный доступ к API через подписку без API key.
//
// Библиотека stateless по дизайну: токен может прийти откуда угодно
// через интерфейс TokenProvider.
//
// Quick start:
//
//	// Из файла токена:
//	client, err := kraube.NewClient(ctx, kraube.WithTokenFile(""))
//
//	// Из токена напрямую:
//	client, err := kraube.NewClient(ctx, kraube.WithToken(token))
//
//	// Из env variable:
//	client, err := kraube.NewClient(ctx, kraube.WithEnvToken("KRAUBE_TOKEN"))
//
//	// Свой провайдер (Vault, Redis, DB, что угодно):
//	client, err := kraube.NewClient(ctx, kraube.WithTokenProvider(myProvider))
//
//	resp, err := client.Messages.Create(ctx, &kraube.MessageRequest{
//	    Model:     kraube.ModelSonnet4_6,
//	    MaxTokens: 1024,
//	    Messages:  []kraube.Message{kraube.UserMessage("Hello!")},
//	})
package kraube
