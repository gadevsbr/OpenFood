# Desenvolvimento e QA

Leia [AGENTS.md](../AGENTS.md), [CONTRIBUTING.md](../CONTRIBUTING.md) e todos os arquivos de `project-memory` antes de alterar o projeto. Use Go 1.27.2; PostgreSQL 17 para integração real. O usuário final não precisa de Node.

## Backend

```sh
go test ./...
go vet ./...
```

Os testes reais exigem `OPENFOOD_TEST_PG_BIN` apontando para os binários PostgreSQL. Testes ignorados não comprovam integração com banco. No Windows, preserve o fallback de caminho 8.3/alias DOS para perfis com acentos.

## Interface e animações

Motion mini 14.1.0, bundle local e reduced-motion:

```powershell
npm.cmd ci --prefix frontend
npm.cmd run build --prefix frontend
```

## Jornadas de navegador

Use exclusivamente uma instalação de teste isolada. Inicie o binário com `OPENFOOD_DATA_DIR` nessa pasta e `OPENFOOD_TEST_HEADLESS=1`; configure `OPENFOOD_UI_TEST_DATA` para a mesma pasta. A jornada inicial exige base vazia. As seguintes dependem de seus fixtures; consulte os pré-requisitos nos scripts antes de executar.

```powershell
npm.cmd install --prefix tools/qa playwright@1.63.0
npx.cmd --prefix tools/qa playwright install chromium
```

Ordem das jornadas incrementais:

1. `node scripts/test-ui-redesign.cjs`: setup/login e fixtures iniciais em base vazia.
2. `node scripts/test-ui-admin.cjs` e `node scripts/test-ui-final.cjs`: administração e regressões gerais.
3. `node scripts/test-ui-sales.cjs`: catálogo/pedidos, usando fixtures iniciais.
4. `node scripts/test-ui-cash.cjs` e `node scripts/test-ui-cash-final.cjs`: recebimentos e caixa.
5. `node scripts/test-ui-pos.cjs`: PDV, após os fixtures de caixa.
6. `node scripts/test-ui-dining.cjs`: mesas/comandas, após PDV/caixa.
7. `node scripts/test-ui-menu.cjs`: cardápio configurável, após PDV/mesas.
8. `node scripts/test-ui-combos.cjs`: combos/clientes, após cardápio.

`OPENFOOD_UI_EVIDENCE` permite direcionar evidências da jornada inicial para a pasta da etapa. Capturas e relatórios só podem ser versionados após sanitização. Consulte os registros de cada etapa em `project-memory` para o escopo validado.

## Instalador e distribuição

`powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-windows.ps1` testa lifecycle em dados isolados. O pipeline `.github/workflows/release.yml` constrói e testa Windows/Linux; releases experimentais incluem checksums e evidências. Não declare validação em máquina limpa, hardware ou provedores externos a partir de build/testes locais.

## Dados e migrações

Nunca use dados de trabalho em QA. Não versione `.env`, dumps, sessões, logs, tokens ou dados pessoais. Gere uma migração nova para cada alteração de esquema; nunca edite migrações aplicadas. Teste upgrade em backup restaurado. Atualize `project-memory` com escopo, decisões e evidências; marque decisões substituídas como `superseded`.
