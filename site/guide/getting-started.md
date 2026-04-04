# Getting Started

## Install

```bash
go get github.com/scott-walker/kraube-api
```

## Authenticate

Build the CLI and log in via OAuth:

```bash
go build -o kraube ./cmd/kraube/
kraube login
```

This opens your browser for OAuth authorization with claude.ai. After confirming, your token is saved to `~/.config/kraube/token`.

## First request

```go
package main

import (
    "context"
    "fmt"
    "log"

    kraube "github.com/scott-walker/kraube-api"
)

func main() {
    ctx := context.Background()

    client, err := kraube.NewClient(ctx, kraube.WithTokenFile(""))
    if err != nil {
        log.Fatal(err)
    }

    resp, err := client.Messages.Create(ctx, &kraube.MessageRequest{
        Model:     kraube.ModelSonnet4_6,
        MaxTokens: 1024,
        Messages:  []kraube.Message{kraube.UserMessage("Hello!")},
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(resp.Text())
}
```

## Other token sources

You don't have to use a token file. See [TokenProvider](./token-provider) for all options:

```go
// Direct token
client, _ := kraube.NewClient(ctx, kraube.WithToken("eyJ..."))

// Environment variable
client, _ := kraube.NewClient(ctx, kraube.WithEnvToken("KRAUBE_TOKEN"))

// Custom provider
client, _ := kraube.NewClient(ctx, kraube.WithTokenProvider(myProvider))
```
