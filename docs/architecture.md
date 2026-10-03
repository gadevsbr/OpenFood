# Arquitetura local
Um binário Go, um núcleo app e PostgreSQL como fonte de verdade. Supervisor Windows gerencia o mesmo app e worker usados no Linux. Interface HTML embutida com código de interação local: não baixa recursos externos. React/TypeScript ainda não adotado neste alpha; UI completa de produto permanece etapa posterior.

`stores` delimita escopo; `users` são administradores da própria loja; sessão identifica loja no servidor. `products` tem preço inteiro em centavos e estoque inteiro em unidades. `orders/order_items` preservam snapshot. Pedidos manuais consomem estoque na confirmação e liberam ao cancelar. Peso, variantes, reservas temporárias e pagamento externo ainda não suportados.

Chave de idempotência por loja + hash do payload rejeita reutilização divergente. Advisory lock serializa repetição da chave; row lock da loja serializa estoque. Pedido, itens, auditoria e job entram na mesma transação. Worker reclama tarefas com SKIP LOCKED e lease de 60s; efeitos locais e conclusão commitam juntos. Nenhuma chamada externa ocorre nessa etapa.

Migração 1 em transação com advisory lock, versão/checksum verificados. Alterar arquivo aplicado é incompatibilidade. Sem promessas de migração transparente para outro banco.

Estados operacionais: confirmed → preparing → ready → completed; confirmed → cancelled. Financeiro separado, manual_pending; não há confirmação por gateway nem botão falso de pago. Snapshot não muda com catálogo. Após restore, sessões são revogadas e jobs pendentes ficam em revisão.

Recursos ainda incompletos, explicitamente fora da evidência deste alpha: RBAC completo, organizações independentes com múltiplas lojas, outbox externa, providers, moedas/quantidades decimais, relatórios completos, auditoria UI, rotações de logs e chaves de segredos externos.
