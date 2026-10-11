# Caixa e recebimentos manuais

1. Um gerente/administrador abre o caixa em **Pagamentos**, informando o dinheiro físico inicial.
2. Em **Pedidos com saldo a receber**, o atendente escolhe o pedido e registra o valor efetivamente recebido. É possível dividir entre dinheiro, cartão conferido e Pix conferido manualmente. Nunca registre apenas uma promessa de pagamento. O sistema não consulta o banco nem a máquina de cartão.
3. O saldo do pedido diminui pelos lançamentos confirmados. O mesmo envio repetido não gera dois lançamentos; o servidor rejeita valor acima do saldo e pedidos cancelados.
4. No fechamento, conte o dinheiro físico. **Esperado = saldo inicial + recebimentos em dinheiro − devoluções em dinheiro no caixa atual**. Cartão/Pix têm totais separados. Diferença é contado menos esperado e exige motivo. Um recebimento durante a contagem exige atualizar e conferir novamente.
5. Para devolver, gerente/administrador primeiro confirma a devolução real ao cliente e depois usa **Devolver** no recebimento, informando motivo. A ação registra a devolução integral daquele recebimento; não executa estorno bancário. Recebimentos já devolvidos não podem ser devolvidos novamente.
6. Uma devolução de recebimento antigo entra no caixa aberto atual, sem reescrever o fechamento anterior. Devolução em dinheiro exige saldo suficiente no caixa atual. Cancelamento só é aceito com saldo recebido zero e quando o estado operacional do pedido permite cancelar.

Atendente recebe; gerente/admin abre, fecha e devolve; consulta financeira somente lê. Não há edição/apagamento de lançamentos pela interface. Data, operador e motivos ficam no histórico e nos backups.

Limites desta versão: sem sangrias, suprimentos, taxas ou devolução parcial de um único recebimento; sem confirmação bancária automática ou contabilidade fiscal. Totais são completos, mas a tela exibe os últimos 100 caixas, 500 movimentos e 200 pedidos pendentes. Os períodos seguem a abertura/fechamento do caixa, podendo atravessar a meia-noite.
