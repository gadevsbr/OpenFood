# Pagamentos manuais e caixa — 2026-10-11

Etapa em execução, ainda sem validação. Skills Payments & Billing Engineer e Bookkeeper & Controller.

Saldo inicial em dinheiro; um caixa aberto por loja; atendente registra recebimentos parciais dinheiro/cartão/Pix manual. Gerente abre/fecha e registra devolução integral de um recebimento com motivo. Lançamentos imutáveis; devolução cria movimento negativo no caixa atual, preservando períodos anteriores. Fechamento compara saldo inicial + recebimentos líquidos em dinheiro com contagem física e exige motivo para diferença. Todas as mutações auditadas e idempotentes; caixa fechado não recebe novos movimentos. Sem sangria/suprimento nesta etapa.

Migração incremental v3→v4 mantém schema v3 original e acrescenta tabelas locais; backup antigo é migrado na base de validação antes de restaurar. Provedores externos continuam indisponíveis. Não equivale a contabilidade fiscal nem confirmação bancária.

Primeira rodada: regressão UI PASS; testes detectaram parâmetros SQL sem tipo na diferença de fechamento e fixture v3 com finais de linha divergentes. Correções: casts bigint e fixture com bytes idênticos ao schema histórico; SQL v4 fixado LF para checksum igual Windows/Linux. Repetição pendente.

Implementação: API local e interface conectadas, separação entre notas de abertura e fechamento, cartão/Pix sempre manuais. Totais usam todos os movimentos; UI limita histórico a 100 caixas/500 movimentos/200 saldos pendentes e informa esses limites. Somente consulta para finance_viewer; cozinha/expedição sem acesso financeiro; atendente recebe mas não abre/fecha/devolve.

Validação intermediária: PostgreSQL completo PASS 114.024s após separar notas; navegador Chromium real regressão e caixa PASS, zero pageerrors, inclusive split 6930 centavos e fechamento esperado 12000/contado 11950/diferença -50, devoluções no período atual e cancelamento após devolução. Falhas anteriores superseded por correção e repetição. Revisão final mobile/escuro e distribuição pendentes. govulncheck sem vulnerabilidades alcançáveis (uma em módulo requerido sem chamadas).

Conferência final: TestCashJourney PASS 25.052s após proteção contra mistura com pedidos de conector; vet/sintaxe/diff PASS. `cash-final-results.json` PASS para notas e totais históricos, desktop/mobile/escuro, atendente e finance_viewer. UI Finish-Gate Reviewer aplicado à inspeção das capturas; ações mobile e dinheiro esperado preservados. Publicação/CI ainda pendentes.
