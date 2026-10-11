# Mesas e comandas — etapa 0.8

Skills: Backend Architect, Payments & Billing Engineer e UX Architect. Em execução, sem declaração de pronto.

Mesas cadastradas por gerente, comanda avulsa ou vinculada à mesa; uma comanda aberta por mesa/identificação na loja. Atendimento cria rodadas como pedidos reais para preparo, com estoque e preços registrados no lançamento. Novos itens não alteram rodadas anteriores. Comandas persistem no PostgreSQL e nos backups.

Fechamento registra recebimentos no caixa atual, distribuídos deterministicamente pelos saldos dos pedidos, sem criar venda agregada e duplicar faturamento. Pagamentos parciais anteriores são descontados do saldo. Preparo continua independente do pagamento; fechar a conta não marca comida como entregue. Idempotência, versão da comanda e saldo esperado protegem concorrência. Não haverá taxa de serviço, desconto de comanda, divisão por pessoa ou transferência de mesa nesta etapa.

UX: acesso Mesas e comandas no PDV, mapa de mesas livres/ocupadas, lista de comandas avulsas e contexto do atendimento junto ao carrinho. Enviar itens para preparo é separado de fechar conta. Comprovante consolidado não fiscal; caixa fechado mantém consulta, mas bloqueia lançamentos e fechamento.

Aceite: PostgreSQL real com concorrência, isolamento, estoque, saldos e rollback; migração v5→v6/backup antigo; Chromium com abrir mesa, duas rodadas, retomada após reload, preparo, pagamento/troco, fechamento/liberação, permissões e mobile. Release experimental com checksums após evidência.

Primeira validação: TestCashJourney PostgreSQL real PASS 23.058s, com duas rodadas, saldo parcial, pagamentos distribuídos, quatro retries simultâneos, permissões, mesa ocupada e backup/restauração. Suíte completa PASS 125.205s, incluindo upgrades v1–v5 e restauração de v3–v5. Chromium mesas PASS com resposta perdida sem duplicar, teclas F4/F8 no contexto, mobile, tema e perfis. Regressões/distribuição ainda pendentes.

Validação local final: base isolada dining-20261011-a, jornadas setup/UI, ADM, consentimentos/offline, caixa, caixa-final, PDV, mesas e vendas PASS; zero pageerrors. TestCashJourney repetido após unicidade de nome sem diferença de caixa PASS 20.923s; vet/diff/sintaxe PASS. Govulncheck zero alcançáveis (uma em módulo requerido sem chamada), npm audit zero. Consulta com caixa fechado, financeiro somente leitura e F4/F8 no contexto verificados. Capturas desktop/mobile/escuro inspecionadas. CI/publicação pendentes.
