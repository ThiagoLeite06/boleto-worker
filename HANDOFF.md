# Handoff — BoletoWorker

> Mostre este arquivo ao Claude no início de uma nova sessão.

## Contexto

Projeto de aprendizado de Go. Claude atua como professor — explica conceitos com analogias antes de implementar, um capítulo por vez.

## Capítulos concluídos

| Cap | Tema | Arquivos principais |
|-----|------|-------------------|
| 1 | Esqueleto Go | `go.mod`, `internal/config/config.go`, `cmd/worker/main.go`, `Makefile` |
| 2 | Consumer RabbitMQ | `internal/consumer/rabbitmq.go` |
| 3 | Docker Compose | `docker-compose.yml` (RabbitMQ + PostgreSQL + Redis) |
| 4 | PostgreSQL | `migrations/001_create_charges.up.sql`, `internal/repository/charge.go` |
| 5 | Idempotência Redis | `internal/idempotency/redis.go` |

## Próximo: Capítulo 6 — Geração de Boleto

Criar `internal/boleto/` com:
- `models.go` — structs `ChargeRequest` e `Boleto`
- `generator.go` — interface `BoletoGenerator`, implementação `APIBoletoGenerator` e `FakeBoletoGenerator`

Config que será adicionada: `BOLETO_API_URL`.

Após o Cap 6, a sequência é:
- **Cap 7** — Retry com backoff exponencial (`internal/retry/backoff.go`)
- **Cap 8** — Processor/orquestrador (`internal/processor/processor.go`) — conecta tudo
- **Cap 9** — Webhook notifier (`internal/notifier/webhook.go`)
- **Cap 10** — Observabilidade (slog + Prometheus + health check)
- **Cap 11** — Graceful shutdown (signal handling + WaitGroup)

## Infraestrutura local

```bash
make up   # sobe RabbitMQ + PostgreSQL + Redis
make run  # roda o worker
```

Containers: `boleto-worker-rabbitmq-1`, `boleto-worker-postgres-1`, `boleto-worker-redis-1`

RabbitMQ UI: http://localhost:15672 (guest/guest)

## Regras da sessão

- Explicar conceito com analogia → confirmar → implementar
- Só adicionar à `Config` o que o capítulo atual usa
- Atualizar `docs/flow.html` ao final de cada capítulo
