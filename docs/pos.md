# PDV e administração

Após login, escolha PDV ou ADM nas duas metades da tela. O botão superior troca o ambiente, preservando as permissões do perfil e o carrinho da aba.

No PDV, caixa fechado exige abertura com o saldo físico inicial. Escolha a categoria e toque nos produtos; ajuste quantidades no carrinho. O padrão é enviar para preparo, acompanhável em ADM → Operação. Balcão conclui a venda imediatamente. Peso, variantes e adicionais ainda não participam deste checkout.

Receber (F4) aceita dinheiro, cartão conferido e Pix manual, inclusive pagamento dividido. A soma deve corresponder ao total; dinheiro entregue determina o troco, mas somente o valor líquido entra no caixa. Desconto exige gerente/admin e motivo. A venda, o estoque e os recebimentos são gravados na mesma transação. Se a resposta se perder, retome a confirmação; a chave da tentativa impede duplicar a venda.

Comprovantes são não fiscais. É possível imprimir pelo navegador e consultar novamente em Vendas e comprovantes. Não há integração bancária, TEF ou emissão NFC-e.

Caixa no topo mostra os saldos, sangria, suprimento e histórico com motivo/operador. Atendentes movimentam e fecham o caixa que abriram; gerente/admin administra qualquer caixa da loja e registra devoluções. Fechamento exige conferência física e motivo para diferença.

F2 busca produto; F8 guarda o carrinho. Até dez carrinhos ficam em espera nesta aba, separados por usuário e loja. Fechar a aba pode apagar estes rascunhos; somente vendas confirmadas estão no banco/backup. Mesas, comandas persistentes, composição de ingredientes, taxas e periféricos físicos permanecem etapas futuras.

ADM abre um resumo real da loja, com vendas de hoje, recebimentos de hoje, pedidos em preparo e saldo a receber. Vendas e recebimentos têm critérios distintos, explicados na tela. Perfis sem consulta financeira recebem apenas informações operacionais.

Motion mini é distribuída localmente e respeita a preferência de movimento reduzido. Evidências de Chromium e PostgreSQL em `docs/evidence/pos-stage/`; dados das capturas são sintéticos.
