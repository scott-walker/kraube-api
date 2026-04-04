# Contributing

## Development

```bash
git clone https://github.com/scott-walker/kraube-api.git
cd kraube-api
go build ./...
go test ./...
```

## Testing

```bash
go vet ./...
go test ./... -race -count=1
```

## Release

Use the `/release` command in Claude Code or follow the [release guide](https://github.com/scott-walker/kraube-api/blob/main/docs/releasing.md).

## Project Rules

- OAuth-only — no API key support
- Stateless authentication via TokenProvider
- Product name is "Kraube API" in prose, `kraube` in code
- Module path: `github.com/scott-walker/kraube-api`

## License

MIT
