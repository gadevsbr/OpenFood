# Clientes: exportação e remoção de identificação

Em **ADM → Clientes**, use **Remover identificação** na linha do cliente ou as
ferramentas de privacidade. Confira os números de pedidos, itens, consentimentos,
recebimentos, vendas do PDV e comandas antes de confirmar no diálogo.

Se os registros mudarem durante a revisão, feche e abra novamente. A remoção é
transacional e auditada: uma falha desfaz a limpeza inteira. Use uma base de teste
para conhecer a operação; os textos removidos não podem ser recuperados pela tela.

| Local | Limpeza da versão 0.15 | O que permanece |
| --- | --- | --- |
| Cadastro do cliente | Nome substituído; contatos, documento, notas e preferência antiga de marketing limpos | ID, datas e indicador de remoção |
| Pedidos e itens vinculados | Cópias de identificação, notas do pedido/item e notas de componentes de combo | Valores, composição, quantidade e estados |
| Consentimentos vinculados | Revogação, origem substituída, evidência e texto livre dos termos limpos | Finalidade, versão, IDs e datas |
| Recebimentos/devoluções e descontos vinculados | Texto livre substituído por marcador de remoção | Centavos, métodos, IDs, datas, relação da devolução e hashes de retries |
| Comandas vinculadas | Rótulo substituído pelo número da comanda; versão atualizada | Rodadas, totais, pagamentos e itens de outros clientes |

**Comandas compartilhadas:** o rótulo é da comanda inteira. Sua substituição afeta
todos os participantes; os pedidos e textos de outros clientes permanecem.
Recebimentos posteriores de pedidos vinculados ao cadastro removido também guardam
um marcador no lugar do texto informado, inclusive nos motivos de devolução.

**Exportação JSON:** reúne os registros vinculados ao cliente na loja selecionada,
incluindo notas de recebimentos, descontos e comandas. O arquivo pode conter dados
pessoais: proteja o download. Não é uma busca por identidade em todos os textos da base.
Uma comanda compartilhada pode ter um rótulo comum, que também aparece na exportação.

**Limites:** textos sem vínculo ao cliente, nomes do catálogo, notas gerais de caixa,
campos livres de finalidade/versão dos termos, operadores e registros externos exigem
revisão própria. IDs/datas/hashes e histórico comercial permanecem; não há garantia
de anonimato. Não coloque identificadores pessoais em campos operacionais gerais.

A limpeza se aplica aos registros vinculados no PostgreSQL. Rascunhos e carrinhos
em espera no `sessionStorage` do navegador, telas já abertas e comprovantes já
impressos também podem conter textos anteriores. Revise e descarte esses dados nas
sessões de atendimento; esta versão não coordena limpeza entre sessões de navegador.

Backups e exportações anteriores não são modificados. Restaurar um backup antigo
pode reintroduzir informações e consentimentos removidos; revise e reaplique os
pedidos de remoção antes de retomar o atendimento. Ainda não há registro externo
de remoções, retenção automática ou prazo fiscal definido pelo programa.

API: `GET /api/customer-privacy-review?customer_id=N` retorna contagens e uma versão.
Envie a versão no header `X-Privacy-Review` ao remover. Versão divergente retorna 409.
As rotas antigas de DELETE continuam compatíveis sem esse header; a UI usa revisão.
Somente administradores/gerentes com gestão de usuários podem revisar e remover.

[Evidências e limites](../project-memory/privacy-stage.md).
