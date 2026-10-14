# Horários de venda

Em **ADM → Catálogo → Categorias**, abra **Horários da loja**. Para restringir uma
categoria, use **Horários** na linha da categoria. O fuso é definido pela loja;
o cadastro da categoria mostra esse fuso em leitura.

Ative **Limitar vendas aos horários abaixo** e adicione faixas por dia. É possível
ter almoço e jantar no mesmo dia. Exemplo: sábado 22:00–02:00 cobre até domingo
às 02:00. Fechar às 00:00 representa meia-noite; 00:00–00:00 cobre o dia inteiro.
A abertura faz parte da faixa; no horário de fechamento ela já está encerrada.
Faixas sobrepostas, inclusive as que atravessam o domingo, são rejeitadas.

- Sem limite: não restringe os horários definidos em outros cadastros.
- Com limite e sem faixas: fechado durante toda a semana.
- Limite de 42 faixas por cadastro. Fusos aceitos nesta etapa: America/* e UTC.
- Uma venda precisa respeitar loja, categoria, categorias superiores e horário
  diário do produto. Categoria inativa também bloqueia seus produtos e descendentes.
- A janela diária antiga do produto é preservada, inclusive os limites inclusivos,
  e passa a usar o fuso da loja. O padrão continua America/Sao_Paulo.

Confira o impacto antes de salvar. Mudanças atualizam as versões do cardápio da
loja; personalize novamente os itens de carrinhos abertos. Se outro operador
alterar o cadastro ou o fuso, reabra o editor antes de tentar salvar.

O PDV mostra **Fora do horário de venda** e bloqueia o botão do produto. Usa a
atualização periódica existente; o servidor confere novamente cada venda, combo
e rodada de comanda, mesmo se a tela estiver desatualizada. Na mudança de horário,
a indisponibilidade prevalece sobre um carrinho antigo.

Pedidos já registrados podem seguir para preparo, recebimento e cancelamento.
O fechamento de horário não cancela pedidos nem fecha o caixa financeiro.
Estoque e preços dos pedidos anteriores são preservados.

A versão continua experimental. [Evidências e limites](../project-memory/schedule-stage.md).
