# Jungle Gaming API

Este projeto foi desenvolvido como parte de um desafio técnico para a Jungle Gaming, utilizando Go para criar uma API de gerenciamento de carteiras e processamento de transações de apostas.

## Sobre o projeto

A aplicação permite criar carteiras de jogadores, consultar saldos e processar apostas, mantendo o controle das movimentações realizadas.

Durante o desenvolvimento, trabalhei com validação de saldo, controle de transações duplicadas, registro das movimentações no banco de dados e autenticação das requisições.

## Tecnologias utilizadas

- Go
- PostgreSQL
- Docker e Docker Compose
- Keycloak
- Uber Fx

## Como executar

Com o Docker Desktop aberto, execute o comando abaixo na pasta do projeto:

```bash
docker compose up --build -d
```

Para verificar se os serviços estão funcionando:

```bash
docker compose ps
```

Para executar os testes:

```bash
go test ./...
```

## Principais rotas

- `GET /health/live` — verifica se a API está respondendo.
- `GET /health/ready` — verifica a conexão com o banco de dados.
- `POST /wallets` — cria uma carteira.
- `GET /wallets/{walletId}` — consulta uma carteira.
- `POST /wagering/transactions` — processa uma transação de aposta.

As requisições precisam de um token Bearer válido. A autenticação é feita utilizando o Keycloak, com o realm `jungle`.

## Banco de dados

O projeto utiliza PostgreSQL para armazenar as informações das carteiras e transações. A estrutura inicial do banco está definida em `migrations/001_init.sql`.

## Considerações finais

O desenvolvimento deste projeto foi uma oportunidade de colocar em prática meus conhecimentos em Go, trabalhar com banco de dados e entender melhor como estruturar uma API com autenticação e controle de transações.

