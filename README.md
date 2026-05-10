# BoletoWorker

Processador assíncrono de cobranças em Go. Consome mensagens de uma fila RabbitMQ, gera boletos via API externa, persiste no PostgreSQL e garante idempotência via Redis.

```
Producer → RabbitMQ → Worker → PostgreSQL
                             → Webhook
```

---

## Stack

| Componente | Tecnologia |
|---|---|
| Linguagem | Go 1.22+ |
| Fila | RabbitMQ (AMQP) |
| Banco | PostgreSQL 16 |
| Cache / Idempotência | Redis 7 |
| Infra local | Docker Compose |

---

## Pré-requisitos

- [Go 1.22+](https://go.dev/dl/)
- [Docker](https://www.docker.com/products/docker-desktop/) e Docker Compose

---

## Começando

### 1. Clone o repositório

```bash
git clone https://github.com/thiagoleite/boleto-worker.git
cd boleto-worker
```

### 2. Suba a infraestrutura

```bash
make up
```

Isso sobe em background:
- **RabbitMQ** na porta `5672` (AMQP) e `15672` (Management UI)
- **PostgreSQL** na porta `5432` — migration aplicada automaticamente
- **Redis** na porta `6379`

### 3. Rode o worker

```bash
make run
```

Saída esperada:

```
BoletoWorker starting...
  log level   : info
  concurrency : 5 workers
  postgres    : conectado
  redis       : conectado
  rabbitmq    : conectado
aguardando mensagens. pressione CTRL+C para sair.
```

---

## Configuração

Copie o arquivo de exemplo e ajuste conforme necessário:

```bash
cp .env.example .env
```

| Variável | Descrição | Default |
|---|---|---|
| `LOG_LEVEL` | Nível de log (`debug`, `info`, `warn`, `error`) | `info` |
| `WORKER_CONCURRENCY` | Número de goroutines processando em paralelo | `5` |
| `RABBITMQ_URL` | URL de conexão com o RabbitMQ | `amqp://guest:guest@localhost:5672/` |
| `POSTGRES_URL` | URL de conexão com o PostgreSQL | `postgres://bw_user:bw_pass@localhost:5432/boletoworker?sslmode=disable` |
| `REDIS_URL` | URL de conexão com o Redis | `redis://localhost:6379/0` |

Para usar o arquivo `.env`:

```bash
export $(cat .env | xargs) && make run
```

---

## Testando manualmente

Com o worker rodando, publique uma mensagem na fila via API HTTP do RabbitMQ:

```bash
curl -s -u guest:guest -X POST \
  http://localhost:15672/api/exchanges/%2F/amq.default/publish \
  -H "Content-Type: application/json" \
  -d '{
    "properties": {},
    "routing_key": "charges.process",
    "payload": "{\"charge_id\":\"chg_001\",\"idempotency_key\":\"idem_001\",\"customer_id\":\"cust_123\",\"customer_name\":\"João Silva\",\"amount_cents\":15000,\"due_date\":\"2025-07-15\"}",
    "payload_encoding": "string"
  }'
```

Resposta esperada: `{"routed":true}`

---

## Formato da mensagem

Cada mensagem publicada na fila `charges.process` deve seguir este contrato:

```json
{
  "charge_id":          "chg_abc123",
  "customer_id":        "cust_456",
  "customer_name":      "João Silva",
  "customer_document":  "12345678901",
  "amount_cents":       15000,
  "due_date":           "2025-07-15",
  "description":        "Parcela 3/12 - Financiamento",
  "idempotency_key":    "idem_xyz789",
  "metadata": {
    "contract_id": "ctr_001"
  }
}
```

| Campo | Tipo | Descrição |
|---|---|---|
| `charge_id` | string | Identificador único da cobrança |
| `idempotency_key` | string | Chave para evitar processamento duplicado |
| `amount_cents` | int | Valor em centavos (ex: `15000` = R$ 150,00) |
| `due_date` | string | Data de vencimento no formato `YYYY-MM-DD` |

---

## Filas

| Fila | Descrição |
|---|---|
| `charges.process` | Fila principal — recebe as cobranças |
| `charges.process.dlq` | Dead Letter Queue — mensagens com falha permanente |

Acesse a interface visual em **http://localhost:15672** (`guest` / `guest`) para monitorar as filas em tempo real.

---

## Comandos disponíveis

```bash
make build        # compila o projeto
make run          # roda o worker
make test         # roda todos os testes
make test-unit    # roda apenas testes unitários
make lint         # verifica o código com go vet
make up           # sobe a infraestrutura Docker
make down         # derruba a infraestrutura Docker
```

---

## Estrutura do projeto

```
boleto-worker/
├── cmd/
│   └── worker/
│       └── main.go           # ponto de entrada
├── internal/
│   ├── config/               # variáveis de ambiente
│   ├── consumer/             # consumer RabbitMQ
│   ├── idempotency/          # controle de duplicatas via Redis
│   ├── repository/           # persistência PostgreSQL
│   ├── processor/            # orquestrador do fluxo
│   ├── boleto/               # integração com API de boletos
│   ├── notifier/             # notificações via webhook
│   └── retry/                # retry com backoff exponencial
├── migrations/               # scripts SQL versionados
├── docs/
│   └── flow.html             # visualização interativa do fluxo
├── docker-compose.yml
├── Makefile
└── .env.example
```

---

## Visualização do fluxo

Abra `docs/flow.html` no navegador para uma visualização interativa e animada do fluxo de processamento — útil para entender como as mensagens percorrem o sistema.

---

## Banco de dados

A tabela `charges` é criada automaticamente pelo Docker Compose na primeira execução. Para inspecionar:

```bash
docker exec boleto-worker-postgres-1 \
  psql -U bw_user -d boletoworker \
  -c "SELECT charge_id, status, attempts, created_at FROM charges;"
```

Para resetar o banco (apaga todos os dados):

```bash
make down
docker volume rm boleto-worker_pgdata
make up
```
