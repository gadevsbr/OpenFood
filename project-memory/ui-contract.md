# Contrato UI/UX — 2026-10-10

Skills: UI Designer, UX Architect e UI Finish-Gate Reviewer (Agency Agents local).
Usuário: atendente acompanha pedidos; gestor mantém catálogo e equipe. Primeira leitura: pedidos ativos e próxima ação. Ação principal: novo pedido manual.

- Identidade: verde floresta, acento lima, fundo mineral e fontes do sistema sem rede externa. Densidade equilibrada, tabelas compactas, valores alinhados e status em português.
- Navegação lateral com loja atual; cabeçalho por área. Catálogo separado de pedidos. Formulários sob demanda com retorno ao contexto. Manutenção e ações destrutivas em área própria.
- Pedidos: busca por número, filtros por estado, contagens derivadas da resposta real (até 200 pedidos), resumo antes da confirmação. Catálogo: busca e disponibilidade.
- Claro/escuro/sistema, foco visível, labels, mensagens anunciadas, prevenção de envio duplicado, estados vazios e erros com recuperação. Mobile preserva número/status/ação e usa navegação recolhível.
- Proibidos: métricas inventadas, integrações apresentadas como operacionais sem prova, gradientes decorativos, formulários extensos antes da lista principal.
- Referências: [Square Order Manager](https://squareup.com/help/us/en/article/6923-pickup-orders-on-square-point-of-sale) (filtros de pedidos); [Shopify catálogo](https://help.shopify.com/en/manual/products/searching-filtering) (busca e filtros); [Lightspeed KDS](https://k-series-support.lightspeedhq.com/hc/en-us/articles/22708154090267-Using-the-Kitchen-Display-System-2-0) (etapas e contagens). Padrões adaptados, sem copiar telas.
- Evidência exigida: navegador com backend PostgreSQL real, setup/login/cadastro/pedido/transições, busca/filtros, teclado, temas, desktop/tablet/mobile, capturas e ausência de erros JavaScript. Não certifica leitor de tela ou ambiente limpo.

Decisão anterior de formulários de cadastro e catálogo na primeira tela: superseded pela fila de pedidos como foco da operação. Bloqueios do backend permanecem registrados na revisão; correções necessárias às jornadas serão identificadas separadamente.
