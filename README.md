# OpenFood
Software livre para operação local de lojas, sem servidor central obrigatório.

Alpha 0.11.0-alpha.1: Windows x64 com PostgreSQL embarcado e Linux com Docker Compose. [Instalação e atualização](docs/installation.md), [plano](project-memory/plan.md), [decisões](project-memory/decisions.md) e [validação](project-memory/validation.md).

Interface de restaurante com categorias visuais, cardápio descritivo e pedido lateral; inspiração de navegação iFood, identidade OpenFood. [Prévia do PDV](docs/evidence/restaurant-ui/pos-desktop.png) e [escopo validado](project-memory/restaurant-ui-stage.md).

Restaurante no PDV: tamanhos/variantes, adicionais obrigatórios e limites, observação por item e fotos locais. Servidor confere preços e três estoques; cozinha, comanda e comprovante preservam as escolhas. [Guia do cardápio](docs/menu.md).

Catálogo: edição de produtos com proteção de estoque concorrente. Pedidos manuais com múltiplos itens e detalhes dos preços registrados na venda; horários de disponibilidade em America/Sao_Paulo.

Caixa: abertura com saldo inicial, recebimentos manuais parciais em dinheiro/cartão/Pix, devolução integral de um recebimento por gerente, fechamento com contagem/diferença/motivo e histórico por operador. Não executa cobrança ou reembolso bancário. Inclui sangrias e suprimentos com motivo/operador; taxas permanecem pendentes. [Uso do caixa](docs/cash.md).

Implementado: setup protegido, login, organizações/lojas e papéis, catálogo, pedidos manuais por unidade/porção, estoque transacional, snapshots de preços, deduplicação e fila local durável. Windows tem supervisor, tray, autostart opt-in, backup/restauração e diagnóstico.

Após login, escolha **PDV | ADM** em duas metades. PDV para restaurante tem produtos/categorias, carrinho, preparo ou balcão, pagamento dividido, troco, desconto gerencial, comprovante não fiscal, carrinhos em espera e caixa obrigatório. ADM abre o dashboard da loja e mantém os cadastros. [Uso do PDV](docs/pos.md). Mesas e comandas persistentes agrupam rodadas de preparo e recebem o saldo sem duplicar vendas; [guia de atendimento](docs/dining.md).

Painel repaginado: fila por etapa, busca e filtros, cadastro contextual, navegação lateral, temas claro/escuro/sistema e layout mobile. [Contrato visual](project-memory/ui-contract.md) e [evidências](docs/evidence/ui-redesign/results.json). Fontes, CSS, JavaScript e ícones são locais, embutidos no executável.

Não representa o produto completo dos requisitos. Existem cadastros de categorias, variantes, adicionais, combos, clientes e consentimentos; ter código não comprova todos os critérios do roadmap. O PDV com variantes/adicionais foi validado na etapa 0.10; Combos foram validados localmente na etapa 0.11; sabores de pizza, pedidos por peso e conectores externos permanecem pendentes. Pix integrado aparece como em desenvolvimento; WhatsApp, iFood e IA não estão conectados. Não há dados fictícios no produto ou telemetria obrigatória. Capturas de QA usam dados de teste identificados.

QA de navegador: `npm.cmd install --prefix tools/qa playwright@1.63.0`, `npx.cmd --prefix tools/qa playwright install chromium`; executar `node scripts/test-ui-redesign.cjs`, `node scripts/test-ui-admin.cjs` e `node scripts/test-ui-final.cjs` com `OPENFOOD_UI_TEST_DATA` apontando para uma instalação de teste isolada, iniciada com `OPENFOOD_TEST_HEADLESS=1`. A primeira suíte exige instalação vazia; as demais usam seus dados. Nunca apontar para dados de trabalho.

QA desta etapa: executar a jornada inicial com `OPENFOOD_UI_EVIDENCE=docs/evidence/sales-stage`, seguida de `node scripts/test-ui-sales.cjs`, no mesmo diretório isolado informado por `OPENFOOD_UI_TEST_DATA`. A jornada de vendas exige os fixtures da primeira suíte.

QA de caixa: em instalação isolada vazia, executar `scripts/test-ui-redesign.cjs` com `OPENFOOD_UI_EVIDENCE=docs/evidence/cash-stage` e depois `node scripts/test-ui-cash.cjs`; ambos usam `OPENFOOD_UI_TEST_DATA`. Testes reais Go exigem `OPENFOOD_TEST_PG_BIN`.

Licença AGPL-3.0-only. O software não exige assinatura comercial; provedores externos opcionais têm seus próprios custos.

Animações: Motion mini 14.1.0, bundle local e reduced-motion; reconstruir com `npm.cmd ci --prefix frontend` e `npm.cmd run build --prefix frontend`. QA PDV: após os fixtures de caixa e caixa-final, executar `node scripts/test-ui-pos.cjs`.

QA mesas/comandas: `node scripts/test-ui-dining.cjs` usa os fixtures do PDV/caixa e exige instalação de teste isolada.

QA cardápio: após as regressões PDV/mesas, `node scripts/test-ui-menu.cjs` usa os mesmos dados isolados e registra evidências em `docs/evidence/menu-stage/`.

Combos no PDV/mesas: preço fixo com ajustes, soma ou maior componente; escolhas por grupo,
estoque dos componentes e composição preservada. Clientes podem ser identificados no PDV;
histórico por loja, consentimentos com texto/versionamento e exportação consistente.
[Guia de combos e clientes](docs/combos-customers.md). Validação local PASS; distribuição 0.11 em conferência.
