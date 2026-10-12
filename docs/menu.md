# Cardápio de restaurante

No ADM, abra **Catálogo → Tamanhos, adicionais e foto** na linha do produto.

- Tamanhos: nome, ajuste em reais e estoque. Produto base R$ 29,90, tamanho
  Grande + R$ 5,00 → R$ 34,90 antes dos adicionais. Use ajuste negativo para
  tamanho mais barato; o preço resultante não pode ser negativo.
- Adicionais: nomeie o grupo, defina mínimo/máximo e marque se é obrigatório.
  Cadastre opções com preço e estoque; uma escolha por opção por unidade do item.
  Desmarque Disponível para suspender tamanho, grupo ou opção.
- Foto: JPEG/PNG de até 2 MB, até 4096 pixels por dimensão e 12 megapixels.
  Recodificada em JPEG, sem metadados; incluída no backup do PostgreSQL.
  Salvar foto preserva alterações ainda não salvas nos tamanhos/adicionais.
- Salve tamanhos/adicionais. Se o estoque ou configuração mudarem durante a edição,
  reabra o cadastro e confira os valores antes de salvar novamente.

No PDV, escolher produto configurado abre tamanho, adicionais, quantidade e
observação por item, como “sem cebola”. O total aparece antes de adicionar.
Use **Personalizar** no carrinho para revisar; escolhas/observações diferentes
ficam em linhas separadas. Tamanhos ativos exigem uma variante disponível.

O estoque é controlado no produto, tamanho e opções. Quantidade 2 consome
2 unidades de cada opção escolhida. Preço e vínculos são conferidos no servidor;
se mudarem, atualize o cardápio e personalize novamente o item.

Balcão e rodadas de mesas usam as mesmas escolhas. Cozinha, conta e comprovante
mantêm nomes, preços e observações registrados na venda. Editar o catálogo não
altera pedidos anteriores. Cancelamento repõe os estoques uma vez; recebimentos
precisam ser devolvidos antes de cancelar um pedido recebido.

Grupos compartilhados são protegidos no editor contextual; ele não altera outros
produtos silenciosamente. Fotos reais são separadas das ilustrações das categorias.

Escopo: unidade/porção, uma variante e seleção única de cada adicional por item.
Combos e sabores de pizza têm guias próprios: [combos](combos-customers.md) e
[pizzas](pizza.md). Peso, estoque por ingrediente e impressora física ficam para
outras etapas. Comprovante não fiscal; pagamentos continuam manuais.
