# OpenFood
Software livre para operação local de lojas, sem servidor central obrigatório.

Alpha 0.5.0-alpha.1: Windows x64 com PostgreSQL embarcado e Linux com Docker Compose. [Instalação e atualização](docs/installation.md), [plano](project-memory/plan.md), [decisões](project-memory/decisions.md) e [validação](project-memory/validation.md).

Catálogo: edição de produtos com proteção de estoque concorrente. Pedidos manuais com múltiplos itens e detalhes dos preços registrados na venda; horários de disponibilidade em America/Sao_Paulo.

Implementado: setup protegido, login, organizações/lojas e papéis, catálogo, pedidos manuais por unidade/porção, estoque transacional, snapshots de preços, deduplicação e fila local durável. Windows tem supervisor, tray, autostart opt-in, backup/restauração e diagnóstico.

Painel repaginado: fila por etapa, busca e filtros, cadastro contextual, navegação lateral, temas claro/escuro/sistema e layout mobile. [Contrato visual](project-memory/ui-contract.md) e [evidências](docs/evidence/ui-redesign/results.json). Fontes, CSS, JavaScript e ícones são locais, embutidos no executável.

Não representa o produto completo dos requisitos. Existem cadastros de categorias, variantes, adicionais, combos, clientes e consentimentos; ter código não comprova todos os critérios do roadmap. Pedidos por peso e conectores externos permanecem pendentes. Pix aparece como em desenvolvimento; WhatsApp, iFood e IA não estão conectados. Não há dados fictícios no produto ou telemetria obrigatória. Capturas de QA usam dados de teste identificados.

QA de navegador: `npm.cmd install --prefix tools/qa playwright@1.63.0`, `npx.cmd --prefix tools/qa playwright install chromium`; executar `node scripts/test-ui-redesign.cjs`, `node scripts/test-ui-admin.cjs` e `node scripts/test-ui-final.cjs` com `OPENFOOD_UI_TEST_DATA` apontando para uma instalação de teste isolada, iniciada com `OPENFOOD_TEST_HEADLESS=1`. A primeira suíte exige instalação vazia; as demais usam seus dados. Nunca apontar para dados de trabalho.

Licença AGPL-3.0-only. O software não exige assinatura comercial; provedores externos opcionais têm seus próprios custos.
