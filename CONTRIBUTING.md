# Contribuição
Leia AGENTS.md e project-memory. Use Go 1.26.8. Rode `go test ./...`; para testes reais Windows, defina OPENFOOD_TEST_PG_BIN para binários PostgreSQL 17. Testes ignorados não são validação de banco.

Antes de mudar esquema, crie migração nova, nunca edite a aplicada. Ensaiar upgrade em backup restaurado e registrar evidência. Não comite .env, dumps, sessões, logs ou segredos. Regras de domínio são compartilhadas entre Windows/Linux. Licença do código original: AGPL-3.0-only.

Instalador/lifecycle: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-windows.ps1` usa somente o pacote e ferramentas nativas Windows em dados de teste isolados. Não use dados de produção.

UI: `npm.cmd install --prefix tools/qa playwright@1.63.0`; `node tools/qa/node_modules/playwright/cli.js install chromium`; inicie o binário com OPENFOOD_DATA_DIR em diretório vazio de teste e `--background`, defina OPENFOOD_UI_TEST_DATA para a mesma pasta e execute `node scripts/test-ui.cjs`. Dados QA e ferramentas ficam ignorados pelo Git; capturas e relatórios podem ser versionados após sanitização. Node não é necessário ao usuário do aplicativo.
