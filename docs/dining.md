# Mesas e comandas

No PDV, abra **Mesas e comandas**. Gerente/admin cadastra as mesas com identificação e quantidade de lugares. Mesa ocupada não pode ser editada ou desativada; nomes e histórico são preservados. Uma mesa pode ter somente uma comanda aberta. Também é possível abrir uma comanda avulsa, sem mesa.

Abra o atendimento e escolha **Adicionar produtos**. O carrinho mostra a identificação da conta. **Enviar itens para preparo** cria uma rodada como pedido real, registra os preços e reserva estoque. Uma observação pode acompanhar a rodada. Na Operação, a cozinha vê a mesa/comanda e a observação. Acrescentar outra rodada não modifica os itens anteriores.

Comandas e rodadas estão no PostgreSQL e no backup. Recarregar a página permite retomar o atendimento. Os itens ainda não enviados continuam sendo rascunhos da aba; feche a rodada para persistir o consumo. F4 envia a rodada no contexto da comanda; F8 não separa estes itens em um carrinho de balcão.

Em **Ver conta → Fechar conta**, confira consumo, recebimento líquido e saldo. Adiantamentos registrados nos pedidos são descontados. O pagamento pode ser dividido entre dinheiro, cartão e Pix manual. O servidor distribui o valor pelos pedidos existentes, sem criar outra venda ou baixar estoque novamente. Troco usa o dinheiro entregue, e apenas o valor devido entra no caixa. Retome a mesma confirmação se a resposta se perder.

Fechamento exige caixa aberto e registra operador. A mesa é liberada após confirmar a conta no banco. Pagar não marca comida como entregue; o preparo continua na Operação. Uma conta vazia pode ser cancelada. Pedidos já enviados usam o cancelamento da Operação, com as regras de estoque e recebimentos existentes.

Financeiro consulta pelo ADM e não altera contas. Atendente abre contas e lança/recebe itens, mas não cadastra mesas. Todos os acessos respeitam a loja atual. Consulta de comandas abertas é limitada a 200 contas, e o histórico mostra as últimas 50 encerradas; até 200 rodadas por conta.

Comprovantes são não fiscais e mostram o consumo, recebido líquido, saldo atual e pagamentos do fechamento. Não há taxa de serviço, desconto de comanda, divisão por pessoa, transferência de mesa, NFC-e ou integração bancária nesta etapa.
