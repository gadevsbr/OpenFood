# Contribuir com o OpenFood

Antes de alterar código, leia [AGENTS.md](AGENTS.md), [README.md](README.md) e todos os arquivos de `project-memory`.

- Use Go 1.27.2. Execute `go test ./...` e `go vet ./...`. Integração real exige PostgreSQL 17 e `OPENFOOD_TEST_PG_BIN`; testes ignorados não validam banco.
- Consulte [Desenvolvimento e QA](docs/development.md) para builds, jornadas de navegador e testes do instalador. Use dados isolados, nunca dados de operação.
- Preserve as regras de domínio compartilhadas entre Windows/Linux. Mudanças de esquema exigem migração nova e ensaio de upgrade em backup restaurado.
- Registre escopo, decisões e evidências em `project-memory`; decisões substituídas ficam como `superseded`.
- Não comite `.env`, dumps, sessões, logs, segredos ou dados pessoais. Capturas devem usar dados sintéticos identificados.

Para bugs, informe versão, sistema operacional, passos para reproduzir e comportamento esperado. Para sugestões, descreva a necessidade de operação e consulte o [roadmap](project-memory/roadmap.md). [Abrir issue](https://github.com/gadevsbr/OpenFood/issues/new/choose).

Pull requests devem explicar o problema, a mudança e a validação executada, incluindo limites. Não declare pronto sem teste ou evidência. O código original é licenciado sob [AGPL-3.0-only](LICENSE).
