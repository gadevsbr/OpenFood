# Combos e clientes

No ADM, Catálogo → Combos: crie o combo e use Itens para vincular produtos.
Editar altera nome, descrição, política/preço e disponibilidade.
Cada componente usa quantidade inteira por unidade/porção. Componentes obrigatórios
entram sempre; opcionais compartilham um grupo, com limites iguais entre suas opções.
Um grupo Bebidas com mínimo/máximo 1 permite escolher uma bebida entre várias.

No PDV, combos aparecem em Todos e na busca. Escolha componentes, tamanhos,
adicionais e observações. A quantidade é de combos; estoques baixam nas quantidades
registradas de cada componente. Combos também entram em rodadas de mesas/comandas.
Produto avulso e combo concorrem pelo mesmo estoque. Cancelar sem recebimento
repõe todos os estoques uma vez; venda recebida exige devolução antes do cancelamento.

Preço fixo: valor do combo + ajustes dos tamanhos/adicionais multiplicados pelas
quantidades dos componentes. Soma: soma dos preços configurados × quantidades.
Maior componente: maior preço configurado × quantidade entre os componentes.
Esta regra não representa divisão de pizza nem média de sabores. Preço e composição
são conferidos no servidor; alteração do combo pede revisão do carrinho.

Cliente no PDV é opcional. Busque nome/telefone e selecione antes de confirmar;
consumidor sem identificação continua disponível. Em mesas, a identificação vale
para a rodada enviada. ADM → Clientes → Histórico mostra os pedidos vinculados
na mesma loja, preservando os valores históricos. Atendimento recebe somente
nome/telefone na busca; exportação e remoção têm permissões administrativas.

Consentimentos registram finalidade, versão, origem, texto dos termos, evidência
opcional e data. Uma finalidade/versão não pode ser sobrescrita: registre nova versão
para novo consentimento. Revogar preserva o registro e a data. Registros antigos
sem texto são mantidos, mas não comprovam os novos requisitos. O indicador legado
marketing_consent é uma preferência antiga; não autoriza comunicação externa.
Não há campanhas/telemetria/conectores externos ativos nesta etapa.

Exportar inclui cadastro, pedidos, itens/combos, consentimentos, pagamentos e
recebimentos locais em uma leitura consistente. Falha de leitura aborta a exportação.
Remover identificação limpa contatos/documento/observações, bloqueia reutilização
do cadastro e revoga consentimentos; mantém valores, composição e histórico financeiro.
IDs/datas permanecem: não é garantia de anonimato nem certificação legal.

Política desta entrega: remoção manual explícita; sem prazo fiscal presumido nem
expiração automática. Backups e JSON exportados anteriormente ainda podem conter
identificadores; não são apagados por esta ação. Restauração de backup anterior pode
reintroduzir dados removidos. Tratamento de retenção e remoção após restauração
continua pendente, assim como sabores de pizza e edição de adicionais compartilhados.

Mapa desta etapa: identificação/contatos/documento/observações em customers;
cópias de identificação e observações em orders; observações em order_items e
selection.combo.components; origem/evidência em consents. Esses campos entram na
remoção. Valores/IDs/datas, produtos/composição, finalidade/versão/texto dos termos,
operadores e motivos financeiros permanecem. Não colocar contatos pessoais em
nomes de produtos, identificação de comanda, texto dos termos ou motivos financeiros.
Revisão desses campos livres, retenção e tombstones em backups continuam no backlog.

## Atualização da política de remoção · 0.15

A descrição anterior que preservava texto dos termos e motivos financeiros está **superseded** pela limpeza ampliada de registros vinculados, com revisão de impacto e preservação dos valores. Sabores de pizza e adicionais compartilhados também já foram entregues nas etapas 0.12/0.13. Consulte o [guia atual de privacidade](privacy.md), inclusive os limites de campos sem vínculo e backups antigos.
