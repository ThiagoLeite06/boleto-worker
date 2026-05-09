# CLAUDE.md — BoletoWorker

> Processador assíncrono de cobranças e boletos em Go.
> Este documento é o guia mestre do projeto. Cada capítulo ensina o conceito, usa analogias pra facilitar, e só depois define o que implementar.

---

## Visão Geral do Projeto

O **BoletoWorker** é um serviço backend em Go que funciona como uma linha de produção numa fábrica de cobranças. Imagine uma esteira industrial:

1. **Entrada** → pedidos de cobrança chegam por uma fila de mensagens (RabbitMQ)
2. **Processamento** → cada pedido é validado, um boleto é gerado, e o status é atualizado
3. **Saída** → o resultado (sucesso ou falha) é notificado via webhook e persistido no banco
4. **Supervisão** → tudo é monitorado com métricas e logs estruturados

O serviço roda N workers em paralelo (goroutines), cada um pegando um item da esteira, processando, e voltando pra pegar o próximo. Se a fábrica precisa desligar (deploy, manutenção), ela termina os itens que estão em andamento antes de parar — isso é **graceful shutdown**.

### Stack

- **Linguagem:** Go 1.22+
- **Fila:** RabbitMQ (AMQP)
- **Banco:** PostgreSQL (via pgx, sem ORM)
- **Cache/Idempotência:** Redis
- **Observabilidade:** slog (logs estruturados) + Prometheus (métricas)
- **Infra local:** Docker Compose
- **Testes:** testcontainers-go (integração) + testing padrão (unitários)

### Estrutura de Diretórios

```
boleto-worker/
├── cmd/
│   └── worker/
│       └── main.go              # Ponto de entrada — monta e liga a fábrica
├── internal/
│   ├── config/
│   │   └── config.go            # Carrega variáveis de ambiente
│   ├── consumer/
│   │   └── rabbitmq.go          # Consome mensagens da fila
│   ├── processor/
│   │   ├── processor.go         # Orquestra a lógica de negócio
│   │   └── processor_test.go
│   ├── boleto/
│   │   ├── generator.go         # Gera boletos (integração com API externa)
│   │   ├── generator_test.go
│   │   └── models.go            # Structs de domínio
│   ├── payment/
│   │   ├── service.go           # Lógica de baixa/conciliação
│   │   └── service_test.go
│   ├── retry/
│   │   ├── backoff.go           # Retry com backoff exponencial
│   │   └── backoff_test.go
│   ├── notifier/
│   │   ├── webhook.go           # Notifica resultado via HTTP
│   │   └── webhook_test.go
│   ├── repository/
│   │   ├── charge.go            # Persistência de cobranças no Postgres
│   │   └── charge_test.go
│   ├── idempotency/
│   │   └── redis.go             # Controle de idempotência via Redis
│   └── observability/
│       ├── logger.go            # Setup de slog
│       └── metrics.go           # Métricas Prometheus
├── migrations/
│   ├── 001_create_charges.up.sql
│   └── 001_create_charges.down.sql
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── go.sum
├── Makefile
└── CLAUDE.md                    # Este arquivo
```

---

## Capítulo 1 — Fundamentos de Go para este Projeto

### 🎓 Antes de Codar: O que você precisa entender

#### 1.1 — Goroutines e Channels (A Cozinha do Restaurante)

**Analogia:** Imagine um restaurante. O **garçom** (goroutine consumer) pega os pedidos dos clientes e coloca num **balcão de pedidos** (channel). Na cozinha, vários **cozinheiros** (goroutines workers) pegam pedidos desse balcão e preparam. Cada cozinheiro trabalha independente. Se o balcão enche, o garçom espera. Se esvazia, os cozinheiros esperam.

```
Cliente → Garçom → [Balcão/Channel] → Cozinheiro 1
                                     → Cozinheiro 2
                                     → Cozinheiro 3
```

**Em Go:**
- `go funcao()` = contratar um novo cozinheiro que trabalha em paralelo
- `ch := make(chan Pedido, 10)` = criar um balcão com espaço pra 10 pedidos
- `ch <- pedido` = garçom coloca pedido no balcão
- `pedido := <-ch` = cozinheiro pega pedido do balcão

**Por que isso importa aqui:** Nosso worker vai ter 1 consumer lendo da fila RabbitMQ e N goroutines processando em paralelo. O channel é a ponte entre eles.

**Diferença do Java:** Em Java você usaria `ExecutorService` com thread pool. Em Go, goroutines são muito mais leves (2KB vs 1MB de uma thread Java). Você pode ter milhares sem problema.

#### 1.2 — Context (O Cronômetro do Jogo)

**Analogia:** Num jogo de futebol, o **árbitro** segura um cronômetro. Quando o tempo acaba, ele apita e todo mundo para. Se ele cancelar o jogo antes (chuva, confusão), todos param também. O `context.Context` em Go é esse cronômetro — ele carrega deadlines, sinais de cancelamento e valores que atravessam toda a cadeia de chamadas.

**Em Go:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel() // sempre chamar cancel pra liberar recursos
```

**Por que isso importa aqui:** Quando o serviço recebe SIGTERM (deploy novo), ele precisa avisar todas as goroutines: "terminem o que estão fazendo em até 30 segundos". O context é o mecanismo pra isso.

**Diferença do Java:** Similar ao `CompletableFuture` com cancel, mas em Go o context é passado explicitamente como primeiro parâmetro de toda função. É convenção da linguagem, não opcional.

#### 1.3 — Interfaces (O Contrato do Encanador)

**Analogia:** Quando você contrata um encanador, não importa se ele é o João ou a Maria — o que importa é que ele sabe **instalar**, **consertar** e **inspecionar**. A interface é esse contrato: qualquer um que cumpra os métodos, serve.

**Em Go:**
```go
type BoletoGenerator interface {
    Generate(ctx context.Context, charge Charge) (Boleto, error)
}
```

Qualquer struct que tenha o método `Generate` com essa assinatura **automaticamente** implementa a interface. Não precisa de `implements` como em Java/Kotlin. Isso se chama **structural typing** (tipagem estrutural).

**Por que isso importa aqui:** Nos testes, vamos trocar o gerador de boleto real por um fake. Na produção, o `APIBoletoGenerator` chama a API externa. Nos testes, o `FakeBoletoGenerator` retorna dados fixos. Os dois satisfazem a mesma interface.

#### 1.4 — Error Handling (O Relatório do Médico)

**Analogia:** Quando um médico examina você, ele não levanta uma exceção e sai correndo. Ele te dá um **relatório** dizendo o que encontrou — normal ou problemático. Em Go, toda função que pode falhar retorna `(resultado, error)`. Você lê o relatório e decide o que fazer.

```go
boleto, err := generator.Generate(ctx, charge)
if err != nil {
    // tratar: logar, retry, retornar erro empacotado
    return fmt.Errorf("falha ao gerar boleto para charge %s: %w", charge.ID, err)
}
```

**O `%w` (error wrapping):** É como colocar um relatório dentro de um envelope com uma nota extra. O erro original fica preservado, mas você adiciona contexto. Quem receber pode "abrir o envelope" com `errors.Is()` ou `errors.As()`.

**Diferença do Java:** Em Java você tem try/catch com exceções que voam pela stack. Em Go, erros são valores retornados. Não existe try/catch. Isso força você a tratar cada erro no ponto exato onde ele acontece.

### 📋 O que Implementar no Capítulo 1

Nada de código de negócio ainda. Este capítulo é só pra montar o esqueleto:

- [ ] Inicializar o módulo Go: `go mod init github.com/seu-user/boleto-worker`
- [ ] Criar a estrutura de diretórios vazia
- [ ] Criar `cmd/worker/main.go` com um `func main()` que só printa "BoletoWorker starting..."
- [ ] Criar `internal/config/config.go` que lê variáveis de ambiente usando `os.Getenv` com defaults
- [ ] Escrever um `Makefile` com targets: `build`, `run`, `test`, `lint`
- [ ] Rodar `go build` e `go run` pra validar que compila

**Variáveis de ambiente esperadas:**
```
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
POSTGRES_URL=postgres://user:pass@localhost:5432/boletoworker?sslmode=disable
REDIS_URL=redis://localhost:6379/0
WORKER_CONCURRENCY=5
WORKER_SHUTDOWN_TIMEOUT=30s
LOG_LEVEL=info
```

---

## Capítulo 2 — A Fila de Mensagens (Consumer RabbitMQ)

### 🎓 Antes de Codar: O que é uma Message Queue

#### 2.1 — Filas de Mensagens (A Caixa de Correio do Prédio)

**Analogia:** Imagine a caixa de correio de um prédio. O **carteiro** (producer) entrega cartas nas caixas. Cada **morador** (consumer) pega as cartas da sua caixa quando quer, no seu ritmo. Se o morador viajou, as cartas ficam acumuladas esperando. Se chegam 100 cartas de uma vez, elas ficam enfileiradas.

Isso é uma **message queue**: um intermediário que desacopla quem envia de quem recebe.

**RabbitMQ** é esse sistema de caixas de correio. Os conceitos-chave:

- **Exchange** = o centro de distribuição dos correios, que decide pra qual caixa vai cada carta
- **Queue** = a caixa de correio específica do nosso serviço
- **Binding** = a regra que liga o exchange à queue ("cartas com etiqueta 'cobrança' vão pra caixa do BoletoWorker")
- **Message** = a carta em si (um JSON com os dados da cobrança)
- **Ack** = o morador confirma que leu a carta. Só aí ela é removida da caixa
- **Nack/Reject** = o morador diz "não consegui processar, devolve" (vai pra retry ou dead letter)

#### 2.2 — Acknowledge e Dead Letter (O Protocolo de Entrega)

**Analogia:** Pense numa transportadora. O entregador traz o pacote e espera você **assinar** (ack). Se você recusa (nack), ele tenta de novo depois ou manda pra um depósito de pacotes não entregues (**Dead Letter Queue — DLQ**).

No nosso worker:
1. Consumer pega mensagem da fila
2. Tenta processar
3. Se sucesso → `Ack` (mensagem removida da fila)
4. Se erro temporário → `Nack` com requeue (volta pra fila, tenta de novo)
5. Se erro permanente (dados inválidos) → `Nack` sem requeue (vai pra DLQ)

**Dead Letter Queue (DLQ):** É o "cemitério" de mensagens que falharam demais. Você pode inspecionar depois pra entender o que deu errado. Toda fila de produção séria precisa de uma DLQ.

#### 2.3 — Prefetch (A Capacidade do Prato)

**Analogia:** Num buffet self-service, o **prefetch** é o tamanho do seu prato. Se seu prato cabe 3 itens, você pega 3 por vez. Se botasse 100 no prato, demoraria pra comer e outros ficariam sem comida.

`prefetchCount = 5` significa: cada consumer pega no máximo 5 mensagens por vez. Quando termina uma, pega outra. Isso evita que um consumer guloso pegue tudo e os outros fiquem ociosos.

### 📋 O que Implementar no Capítulo 2

- [ ] Adicionar dependência: `github.com/rabbitmq/amqp091-go`
- [ ] Criar `internal/consumer/rabbitmq.go` com:
  - Struct `RabbitMQConsumer` que mantém conexão e channel AMQP
  - Método `Connect(ctx context.Context) error` — conecta e declara queue + DLQ
  - Método `Consume(ctx context.Context) (<-chan amqp.Delivery, error)` — retorna channel de mensagens
  - Método `Close() error` — fecha conexão limpa
- [ ] A queue principal deve se chamar `charges.process`
- [ ] A DLQ deve se chamar `charges.process.dlq`
- [ ] Configurar prefetch = valor de `WORKER_CONCURRENCY`

**Contrato da mensagem (JSON):**
```json
{
  "charge_id": "chg_abc123",
  "customer_id": "cust_456",
  "customer_name": "João Silva",
  "customer_document": "12345678901",
  "amount_cents": 15000,
  "due_date": "2025-07-15",
  "description": "Parcela 3/12 - Financiamento Veículo",
  "idempotency_key": "idem_xyz789",
  "metadata": {
    "vehicle_plate": "ABC1D23",
    "contract_id": "ctr_001"
  }
}
```

**Struct Go correspondente** (criar em `internal/boleto/models.go`):
```go
type ChargeRequest struct {
    ChargeID         string            `json:"charge_id"`
    CustomerID       string            `json:"customer_id"`
    CustomerName     string            `json:"customer_name"`
    CustomerDocument string            `json:"customer_document"`
    AmountCents      int64             `json:"amount_cents"`
    DueDate          string            `json:"due_date"`
    Description      string            `json:"description"`
    IdempotencyKey   string            `json:"idempotency_key"`
    Metadata         map[string]string `json:"metadata,omitempty"`
}
```

---

## Capítulo 3 — Persistência (Repository Pattern com PostgreSQL)

### 🎓 Antes de Codar: Banco de Dados sem ORM

#### 3.1 — Por que sem ORM em Go (O Mecânico vs. o Botão Mágico)

**Analogia:** Um ORM é como um carro automático com piloto automático. Você aperta um botão e ele faz tudo. Mas quando o piloto automático erra (e vai errar), você não sabe dirigir manual. Em Go, a comunidade prefere o **mecânico** — alguém que entende o motor (SQL puro) e usa ferramentas que auxiliam sem esconder o que está acontecendo.

**pgx** é o driver PostgreSQL mais usado em Go. Ele te dá:
- Connection pooling nativo
- Suporte a tipos PostgreSQL nativos (UUID, JSONB, arrays)
- Prepared statements automáticos
- Controle total das queries

Você escreve SQL puro e mapeia os resultados pra structs Go manualmente. Parece mais trabalho, mas você sabe exatamente o que cada query faz, e debugar é trivial.

#### 3.2 — Migrations (A Planta da Casa)

**Analogia:** Antes de construir uma casa, você faz a **planta**. Migrations são as plantas do banco de dados — cada uma descreve uma alteração (criar tabela, adicionar coluna). Elas são versionadas e aplicadas em ordem, como andares de um prédio: o 2º andar só existe depois do 1º.

- **up** = construir o andar
- **down** = demolir o andar (rollback)

#### 3.3 — Connection Pool (A Recepção do Hotel)

**Analogia:** Um hotel tem 50 quartos. Quando um hóspede chega, a recepção dá uma chave. Quando sai, devolve. O **connection pool** é essa recepção: mantém N conexões prontas com o banco. Quando uma goroutine precisa, pega uma. Quando termina, devolve. Criar conexão nova a cada request seria como construir um quarto novo pra cada hóspede — lento e caro.

Configurações importantes:
- `MaxConns` = número máximo de quartos (conexões)
- `MinConns` = quartos sempre prontos, mesmo sem hóspedes
- `MaxConnLifetime` = tempo máximo que um quarto fica sem reforma (recicla conexões velhas)

### 📋 O que Implementar no Capítulo 3

- [ ] Adicionar dependência: `github.com/jackc/pgx/v5/pgxpool`
- [ ] Criar migration `migrations/001_create_charges.up.sql`:

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE charge_status AS ENUM (
    'pending',
    'processing',
    'boleto_generated',
    'paid',
    'failed',
    'cancelled'
);

CREATE TABLE charges (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    charge_id       VARCHAR(100) UNIQUE NOT NULL,
    customer_id     VARCHAR(100) NOT NULL,
    customer_name   VARCHAR(255) NOT NULL,
    customer_doc    VARCHAR(14) NOT NULL,
    amount_cents    BIGINT NOT NULL,
    due_date        DATE NOT NULL,
    description     TEXT,
    status          charge_status NOT NULL DEFAULT 'pending',
    boleto_url      TEXT,
    boleto_barcode  VARCHAR(60),
    attempts        INT NOT NULL DEFAULT 0,
    max_attempts    INT NOT NULL DEFAULT 5,
    last_error      TEXT,
    metadata        JSONB,
    idempotency_key VARCHAR(100) UNIQUE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at    TIMESTAMPTZ
);

CREATE INDEX idx_charges_status ON charges(status);
CREATE INDEX idx_charges_customer ON charges(customer_id);
CREATE INDEX idx_charges_due_date ON charges(due_date);
```

- [ ] Criar migration `migrations/001_create_charges.down.sql`:
```sql
DROP TABLE IF EXISTS charges;
DROP TYPE IF EXISTS charge_status;
```

- [ ] Criar `internal/repository/charge.go` com:

```go
type ChargeRepository interface {
    Upsert(ctx context.Context, charge *Charge) error
    UpdateStatus(ctx context.Context, chargeID string, status ChargeStatus, opts ...UpdateOption) error
    GetByChargeID(ctx context.Context, chargeID string) (*Charge, error)
    GetPendingRetries(ctx context.Context, limit int) ([]Charge, error)
}
```

- [ ] Implementar `PostgresChargeRepository` que satisfaz a interface usando pgxpool
- [ ] Cada método deve usar query parametrizada (nunca concatenar SQL)
- [ ] `Upsert` deve usar `ON CONFLICT (idempotency_key) DO UPDATE` pra garantir idempotência no banco

---

## Capítulo 4 — Idempotência (Redis como Guardião)

### 🎓 Antes de Codar: O que é Idempotência

#### 4.1 — Idempotência (O Botão do Elevador)

**Analogia:** Quando você aperta o botão do elevador, não importa se aperta 1 vez ou 50 — o elevador vem uma vez só. Isso é **idempotência**: executar a mesma operação múltiplas vezes produz o mesmo resultado que executar uma vez.

**Por que isso é crítico aqui:** Em sistemas distribuídos, mensagens podem ser entregues mais de uma vez (at-least-once delivery). Se o worker processa uma cobrança e o ack se perde, o RabbitMQ reenvia. Sem idempotência, o cliente recebe dois boletos. Com idempotência, o segundo processamento detecta "já fiz isso" e ignora.

#### 4.2 — Redis como Trava Rápida (O Crachá da Catraca)

**Analogia:** Numa catraca de metrô, você encosta o crachá e passa. Se encostar de novo em 1 segundo, a catraca bloqueia — já registrou sua entrada. O Redis funciona como essa catraca: rápido, em memória, e com TTL (tempo de expiração).

**Estratégia:**
```
SET idempotency:{key} "processing" NX EX 3600
```
- `NX` = só seta se a chave **não existe** (atômico, sem race condition)
- `EX 3600` = expira em 1 hora (limpeza automática)

Se `SET` retorna `true` → primeira vez, processa.
Se `SET` retorna `false` → duplicata, ignora.

#### 4.3 — Dupla Barreira (Cinto e Airbag)

A idempotência no Redis é o **cinto de segurança** — rápido e pega a maioria dos casos. O `UNIQUE` constraint no Postgres via `idempotency_key` é o **airbag** — backup pro caso raro do Redis falhar. As duas camadas juntas dão segurança real.

### 📋 O que Implementar no Capítulo 4

- [ ] Adicionar dependência: `github.com/redis/go-redis/v9`
- [ ] Criar `internal/idempotency/redis.go` com:

```go
type IdempotencyChecker interface {
    TryAcquire(ctx context.Context, key string, ttl time.Duration) (acquired bool, err error)
    Release(ctx context.Context, key string) error
    MarkCompleted(ctx context.Context, key string, ttl time.Duration) error
}
```

- [ ] Implementar `RedisIdempotencyChecker` usando `SET NX EX`
- [ ] Os estados da chave devem ser: `"processing"` (adquirida) → `"completed"` (finalizada)
- [ ] Se Redis está fora do ar, o checker deve retornar `acquired=true` com log de warning (fail-open) — a barreira do Postgres ainda protege
- [ ] TTL padrão: 1 hora pra processing, 24 horas pra completed

---

## Capítulo 5 — Geração de Boleto (Integração com API Externa)

### 🎓 Antes de Codar: Integrações Externas em Go

#### 5.1 — HTTP Client Robusto (O Telefone Vermelho)

**Analogia:** Imagine que você tem um **telefone vermelho** pra ligar pro banco e pedir um boleto. Às vezes o banco demora pra atender (timeout), às vezes a linha cai (erro de rede), às vezes ele diz "estou ocupado, liga depois" (rate limit). Seu código precisa lidar com tudo isso.

Regras de ouro pra HTTP clients em Go:
- **Sempre** definir timeout no `http.Client` (nunca usar o default que é infinito)
- **Sempre** fechar o `resp.Body` com `defer resp.Body.Close()`
- **Reusar** o client (ele tem connection pool interno)
- **Verificar** status codes (200 ≠ "deu certo" pra todas as APIs)

#### 5.2 — Circuit Breaker (O Disjuntor da Casa)

**Analogia:** O disjuntor elétrico da sua casa desliga quando tem sobrecarga, protegendo os aparelhos. Depois de um tempo, você tenta religar. O **circuit breaker** no software faz o mesmo: se a API externa falha 5 vezes seguidas, ele "desarma" e para de tentar por um tempo, evitando cascata de erros.

Estados:
- **Closed** (normal) → requisições passam
- **Open** (desarmado) → requisições são rejeitadas imediatamente, sem chamar a API
- **Half-Open** (testando) → uma requisição passa pra testar se a API voltou

Neste projeto, não vamos implementar circuit breaker completo no Capítulo 5, mas a interface já vai suportar isso no futuro.

### 📋 O que Implementar no Capítulo 5

- [ ] Criar `internal/boleto/generator.go` com:

```go
type BoletoGenerator interface {
    Generate(ctx context.Context, req ChargeRequest) (*Boleto, error)
}

type Boleto struct {
    URL        string    `json:"url"`
    Barcode    string    `json:"barcode"`
    ExpiresAt  time.Time `json:"expires_at"`
}
```

- [ ] Implementar `APIBoletoGenerator` que faz HTTP POST pra uma API externa (simulada)
  - Timeout de 10 segundos no http.Client
  - Retry simples (usar o módulo de retry do Capítulo 6)
  - Logging de request/response
  - Tratamento de erros por status code (400 = erro permanente, 500 = erro temporário)

- [ ] Implementar `FakeBoletoGenerator` pra testes:
  - Retorna boleto com dados fixos
  - Aceita opção pra simular falha

- [ ] A "API externa" será um endpoint fake definido em variável de ambiente (`BOLETO_API_URL`). Pra dev local, vamos criar um mock server simples no Docker Compose.

---

## Capítulo 6 — Retry com Backoff Exponencial

### 🎓 Antes de Codar: A Arte de Tentar de Novo

#### 6.1 — Backoff Exponencial (O Vizinho Barulhento)

**Analogia:** Seu vizinho está fazendo obra. Você vai lá pedir pra parar:
- 1ª tentativa: espera **1 segundo**, bate na porta
- 2ª tentativa: espera **2 segundos**, bate de novo
- 3ª tentativa: espera **4 segundos**
- 4ª tentativa: espera **8 segundos**
- 5ª tentativa: espera **16 segundos** (já tá irritado, desiste)

O tempo dobra a cada tentativa. Isso é **backoff exponencial**. A ideia é não sobrecarregar um serviço que já está com problemas.

**Fórmula:** `delay = baseDelay * 2^tentativa`

#### 6.2 — Jitter (O Trânsito no Semáforo)

**Analogia:** Quando o semáforo abre, se todos os carros acelerarem ao mesmo tempo, tem engarrafamento. Se cada um esperar um tempo aleatório (0.1s, 0.3s, 0.5s...), o fluxo é suave. O **jitter** adiciona uma variação aleatória ao delay pra evitar que todos os workers retentem ao mesmo tempo (thundering herd problem).

**Fórmula com jitter:** `delay = baseDelay * 2^tentativa + random(0, baseDelay)`

#### 6.3 — Erros Retentáveis vs. Permanentes

Nem todo erro merece retry:
- **Retentável:** timeout, erro 500, erro 503, conexão recusada → o serviço pode estar temporariamente indisponível
- **Permanente:** erro 400 (dados inválidos), erro 404 (recurso não existe), erro de validação → tentar de novo não vai resolver nada

O worker precisa distinguir os dois. Erros permanentes vão direto pra DLQ.

### 📋 O que Implementar no Capítulo 6

- [ ] Criar `internal/retry/backoff.go` com:

```go
type RetryConfig struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
    WithJitter  bool
}

type RetryableError struct {
    Err       error
    Retryable bool
}

func Do(ctx context.Context, cfg RetryConfig, fn func(attempt int) error) error
```

- [ ] A função `Do` deve:
  - Executar `fn` até `MaxAttempts` vezes
  - Esperar com backoff exponencial + jitter entre tentativas
  - Respeitar cancelamento do context (parar se ctx foi cancelado)
  - Distinguir `RetryableError` — se `Retryable=false`, parar imediatamente
  - Cap no `MaxDelay` (nunca esperar mais que, ex, 1 minuto)
  - Logar cada tentativa com nível e attempt number

- [ ] Defaults recomendados:
  - `MaxAttempts = 5`
  - `BaseDelay = 1s`
  - `MaxDelay = 30s`
  - `WithJitter = true`

---

## Capítulo 7 — O Processador (Orquestrador Central)

### 🎓 Antes de Codar: O Maestro da Orquestra

#### 7.1 — O Processor como Maestro

**Analogia:** Numa orquestra, o **maestro** não toca nenhum instrumento. Ele coordena: "violinos, comecem. Agora flautas. Percussão, esperem o compasso 32." O **Processor** é nosso maestro — ele não gera boleto, não salva no banco, não envia notificação. Ele **coordena** quem faz o quê e em que ordem.

Fluxo do Processor:
```
1. Recebe ChargeRequest
2. Verifica idempotência (Redis) → já processou? → ignora
3. Persiste/atualiza no banco (status = processing)
4. Gera boleto (BoletoGenerator)
5. Atualiza banco (status = boleto_generated, salva URL)
6. Notifica via webhook (Notifier)
7. Atualiza banco (status = completed)
8. Marca idempotência como completed
```

Se qualquer passo falha:
- Erro retentável → nack com requeue
- Erro permanente → atualiza banco (status = failed), nack sem requeue (DLQ)

#### 7.2 — Dependency Injection em Go (A Tomada Universal)

**Analogia:** Uma tomada universal aceita qualquer plugue — americano, europeu, inglês. O Processor aceita qualquer implementação das interfaces: qualquer `BoletoGenerator`, qualquer `ChargeRepository`, qualquer `Notifier`. Na produção, plugamos os reais. Nos testes, plugamos os fakes.

Em Go, DI é feito pelo **construtor**:
```go
func NewProcessor(
    repo    repository.ChargeRepository,
    gen     boleto.BoletoGenerator,
    idem    idempotency.IdempotencyChecker,
    notify  notifier.Notifier,
    retry   retry.RetryConfig,
    logger  *slog.Logger,
) *Processor
```

Sem framework de DI (Spring, Dagger). Sem magia. É só passagem de parâmetro.

### 📋 O que Implementar no Capítulo 7

- [ ] Criar `internal/processor/processor.go` com:

```go
type Processor struct {
    repo     repository.ChargeRepository
    gen      boleto.BoletoGenerator
    idem     idempotency.IdempotencyChecker
    notify   notifier.Notifier
    retryCfg retry.RetryConfig
    logger   *slog.Logger
}

func (p *Processor) Process(ctx context.Context, req boleto.ChargeRequest) error
```

- [ ] O método `Process` deve seguir o fluxo descrito acima (7 passos)
- [ ] Cada passo deve ser logado com slog (nível info/error, com charge_id no contexto)
- [ ] Erros devem ser classificados (retentável vs. permanente) e propagados adequadamente
- [ ] Testes unitários usando fakes de todas as dependências

---

## Capítulo 8 — Notificações (Webhook)

### 🎓 Antes de Codar: Webhooks

#### 8.1 — Webhook (A Campainha da Pizza)

**Analogia:** Quando você pede pizza, o entregador **toca a campainha** (webhook) quando chega. Você não precisa ficar abrindo a porta a cada 5 minutos pra ver se chegou (polling). O webhook é um HTTP POST que o nosso serviço faz pra um endpoint configurado pelo cliente, avisando: "o boleto foi gerado" ou "a cobrança falhou".

O payload do webhook deve ser assinado (HMAC-SHA256) pra que o receptor possa verificar que veio do nosso serviço e não de um impostor.

### 📋 O que Implementar no Capítulo 8

- [ ] Criar `internal/notifier/webhook.go` com:

```go
type Notifier interface {
    Notify(ctx context.Context, event NotificationEvent) error
}

type NotificationEvent struct {
    EventType string      `json:"event_type"` // "boleto.generated", "charge.failed"
    ChargeID  string      `json:"charge_id"`
    Payload   interface{} `json:"payload"`
    Timestamp time.Time   `json:"timestamp"`
}
```

- [ ] Implementar `WebhookNotifier`:
  - HTTP POST pra URL configurada via env (`WEBHOOK_URL`)
  - Timeout de 5 segundos
  - Header `X-Signature` com HMAC-SHA256 do body usando secret configurada
  - Retry (2 tentativas) em caso de erro temporário
  - Se webhook falha, logar mas **não** falhar o processamento (fire-and-forget tolerante)

---

## Capítulo 9 — Observabilidade (Logs e Métricas)

### 🎓 Antes de Codar: Os 3 Pilares da Observabilidade

#### 9.1 — Logs Estruturados (O Diário Organizado)

**Analogia:** Um diário desorganizado diz "hoje foi ruim". Um diário estruturado diz: "data=2025-07-15 humor=ruim motivo=chuva atividade=ficar_em_casa". Logs estruturados (JSON) permitem filtrar, buscar e agregar por qualquer campo. Em vez de `log.Println("erro ao processar cobrança")`, escrevemos:

```go
logger.Error("falha ao gerar boleto",
    "charge_id", req.ChargeID,
    "attempt", attempt,
    "error", err.Error(),
    "duration_ms", elapsed.Milliseconds(),
)
```

Go 1.21+ tem o pacote `slog` na stdlib — não precisa de lib externa.

#### 9.2 — Métricas (O Painel do Carro)

**Analogia:** O painel do carro mostra velocidade, RPM, temperatura, combustível — tudo em tempo real. Métricas fazem o mesmo pro serviço. Com Prometheus, exportamos:

- `charges_processed_total` (counter) — quantas cobranças processamos, por status
- `charge_processing_duration_seconds` (histogram) — quanto tempo cada processamento leva
- `charges_in_flight` (gauge) — quantas cobranças estão sendo processadas agora
- `boleto_generation_errors_total` (counter) — quantas vezes a API de boleto falhou

#### 9.3 — Health Check (O Estetoscópio)

O endpoint `/health` é o estetoscópio do serviço. Kubernetes, load balancers e monitoring tools usam ele pra saber se o serviço está vivo e saudável.

- `/health/live` → "estou rodando?" (liveness)
- `/health/ready` → "consigo processar?" (readiness — testa conexão com Rabbit, Postgres, Redis)

### 📋 O que Implementar no Capítulo 9

- [ ] Criar `internal/observability/logger.go`:
  - Factory function que cria `*slog.Logger` com nível configurável
  - Output em JSON pra produção
  - Adicionar campos padrão: `service=boleto-worker`, `version=x.y.z`

- [ ] Criar `internal/observability/metrics.go`:
  - Dependência: `github.com/prometheus/client_golang`
  - Registrar as 4 métricas listadas acima
  - Expor endpoint `/metrics` via HTTP separado (porta 9090)

- [ ] Criar health check HTTP:
  - `/health/live` → sempre 200
  - `/health/ready` → pinga Rabbit, Postgres, Redis. 200 se todos ok, 503 se algum falhou

---

## Capítulo 10 — Orquestração Final (main.go e Graceful Shutdown)

### 🎓 Antes de Codar: Ligando Tudo

#### 10.1 — Graceful Shutdown (O Pouso do Avião)

**Analogia:** Um avião não para no ar. Quando vai pousar, o piloto reduz velocidade, alinha com a pista, desce suavemente e só desliga os motores depois de parar completamente. **Graceful shutdown** é o pouso do nosso serviço:

1. Recebe sinal de parada (SIGTERM/SIGINT)
2. Para de consumir novas mensagens da fila
3. Espera os workers terminarem o que estão fazendo (com timeout)
4. Fecha conexões (Rabbit, Postgres, Redis)
5. Desliga

Se o timeout estoura, o serviço faz **hard shutdown** — mata tudo. Melhor perder uma mensagem (ela volta pra fila via nack) do que ficar pendurado pra sempre.

#### 10.2 — WaitGroup (O Chamada na Escola)

**Analogia:** O professor faz chamada antes de sair pra excursão. Cada aluno (goroutine) que entra no ônibus faz `wg.Add(1)`. Quando desce, faz `wg.Done()`. O professor espera com `wg.Wait()` — só parte quando todos estão dentro ou fora.

```go
var wg sync.WaitGroup
for i := 0; i < concurrency; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        // processar mensagens até ctx ser cancelado
    }()
}
wg.Wait() // espera todos os workers terminarem
```

#### 10.3 — Signal Handling (O Alarme de Incêndio)

Quando o sistema operacional manda SIGTERM (deploy) ou SIGINT (Ctrl+C), o Go captura com `signal.Notify`. É como o alarme de incêndio: quando toca, todos seguem o protocolo de evacuação (shutdown).

### 📋 O que Implementar no Capítulo 10

- [ ] Reescrever `cmd/worker/main.go` com:

```go
func main() {
    // 1. Carregar config
    // 2. Criar logger
    // 3. Conectar Postgres (pool)
    // 4. Conectar Redis
    // 5. Conectar RabbitMQ
    // 6. Criar dependências (repo, generator, checker, notifier)
    // 7. Criar Processor
    // 8. Iniciar servidor de métricas/health (goroutine)
    // 9. Iniciar consumer
    // 10. Iniciar N workers (goroutines)
    // 11. Esperar sinal de shutdown
    // 12. Graceful shutdown com timeout
}
```

- [ ] O loop de cada worker:
```go
for {
    select {
    case <-ctx.Done():
        return // shutdown
    case msg, ok := <-deliveries:
        if !ok {
            return // channel fechou
        }
        // deserializar, chamar processor.Process, ack/nack
    }
}
```

- [ ] Signal handling com `signal.NotifyContext`
- [ ] Shutdown com timeout configurável (default 30s)
- [ ] Ordem de shutdown: parar consumer → esperar workers → fechar Postgres → fechar Redis → fechar Rabbit

---

## Capítulo 11 — Infraestrutura Local (Docker Compose)

### 🎓 Antes de Codar: Container como Ambiente

#### 11.1 — Docker Compose (A Maquete da Cidade)

**Analogia:** Antes de construir uma cidade real, arquitetos fazem uma **maquete**. Docker Compose é a maquete do seu ambiente de produção: cada serviço (Postgres, Redis, RabbitMQ, mock API) é um prédio na maquete, com endereços internos e redes conectando tudo.

### 📋 O que Implementar no Capítulo 11

- [ ] Criar `docker-compose.yml` com:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: boletoworker
      POSTGRES_USER: bw_user
      POSTGRES_PASSWORD: bw_pass
    ports: ["5432:5432"]
    volumes: ["pgdata:/var/lib/postgresql/data"]

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]

  rabbitmq:
    image: rabbitmq:3-management-alpine
    ports:
      - "5672:5672"
      - "15672:15672"
    environment:
      RABBITMQ_DEFAULT_USER: guest
      RABBITMQ_DEFAULT_PASS: guest

  mock-boleto-api:
    build: ./mock-api
    ports: ["8081:8081"]

volumes:
  pgdata:
```

- [ ] Criar `mock-api/` com um servidor Go simples que simula a API de geração de boletos:
  - `POST /boletos` → retorna boleto fake com 200 (90% das vezes) ou 500 (10%, pra testar retry)
  - Delay aleatório de 100-500ms pra simular latência real

- [ ] Criar `Dockerfile` multi-stage pro worker:
  - Stage 1: `golang:1.22-alpine` → build
  - Stage 2: `alpine:3.19` → runtime (binário só)

- [ ] Adicionar target no Makefile: `make up` (docker-compose up) e `make down`

---

## Capítulo 12 — Testes

### 🎓 Antes de Codar: Estratégia de Testes em Go

#### 12.1 — A Pirâmide de Testes (A Fundação da Casa)

**Analogia:** Uma casa tem fundação (muitos tijolos pequenos), paredes (menos, maiores) e telhado (poucos, grandes). Testes seguem a mesma pirâmide:

- **Base: Testes unitários** → rápidos, isolados, muitos. Testam funções puras e lógica com fakes.
- **Meio: Testes de integração** → testam componentes com dependências reais (Postgres, Redis via testcontainers).
- **Topo: Testes E2E** → testam o fluxo completo (publicar mensagem → verificar resultado no banco). Poucos e lentos.

#### 12.2 — Testcontainers (O Laboratório Portátil)

**Analogia:** Em vez de testar um remédio num hospital inteiro, o cientista monta um **laboratório portátil** com tudo que precisa: microscópio, reagentes, amostras. `testcontainers-go` sobe um Postgres real, um Redis real, um Rabbit real — tudo em containers Docker, só pro tempo do teste.

### 📋 O que Implementar no Capítulo 12

- [ ] Adicionar dependência: `github.com/testcontainers/testcontainers-go`
- [ ] **Unitários** (cada módulo):
  - `processor_test.go` → testar fluxo completo com fakes
  - `backoff_test.go` → testar cálculo de delays e jitter
  - `generator_test.go` → testar parsing de resposta e tratamento de erros

- [ ] **Integração**:
  - `charge_test.go` → testar Upsert, UpdateStatus, GetPendingRetries com Postgres real
  - `redis_test.go` → testar TryAcquire, MarkCompleted com Redis real

- [ ] **E2E** (opcional, Capítulo bônus):
  - Subir todo o ambiente, publicar mensagem no Rabbit, esperar processamento, verificar resultado no Postgres

- [ ] Configurar CI no Makefile: `make test-unit`, `make test-integration`, `make test-all`

---

## Roadmap de Implementação

| Fase | Capítulos | Objetivo | Estimativa |
|------|-----------|----------|------------|
| 1 - Fundação | 1, 11 | Esqueleto + infra local rodando | 1-2 dias |
| 2 - Dados | 2, 3, 4 | Consumer + Banco + Idempotência | 3-4 dias |
| 3 - Negócio | 5, 6, 7 | Boleto + Retry + Processador | 3-4 dias |
| 4 - Produção | 8, 9, 10 | Notificação + Observabilidade + Shutdown | 2-3 dias |
| 5 - Qualidade | 12 | Testes unitários + integração | 2-3 dias |

**Total estimado: 2-3 semanas em ritmo de side project.**

---

## Convenções do Projeto

- **Nomenclatura:** snake_case pra arquivos, camelCase pra variáveis, PascalCase pra tipos exportados
- **Erros:** sempre empacotar com `fmt.Errorf("contexto: %w", err)`
- **Logs:** slog com campos estruturados, sempre incluir `charge_id` quando disponível
- **Contexto:** toda função que faz I/O recebe `context.Context` como primeiro parâmetro
- **Testes:** arquivos `_test.go` no mesmo pacote. Table-driven tests quando fizer sentido
- **Git:** conventional commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`)
- **Branches:** `feat/{capitulo}-{descricao}` (ex: `feat/cap2-rabbitmq-consumer`)
