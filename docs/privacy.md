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

## Rascunhos e telas de atendimento

A remoção altera a revisão de privacidade da loja. As abas da versão 0.16 verificam
essa revisão antes das chamadas à API, a cada três segundos e ao retornar/reconectar.
Quando muda, descartam **todos os rascunhos, carrinhos em espera e tentativas locais
do PDV daquela aba** e recarregam a tela, incluindo dados em memória e formulários.
O descarte conservador também pode afetar vendas de outros clientes na mesma aba.
Na primeira abertura após atualizar, rascunhos antigos sem revisão são descartados.
Logout limpa rascunhos da aba.

Sem conexão, uma tela modal cobre os dados e bloqueia teclado/interação até a
verificação. Uma aba fechada/suspensa só executa código quando volta a funcionar;
ela deve se reconectar antes do atendimento. Reabra abas antigas após atualizar.
Não há apagamento remoto de um navegador desligado nem de arquivos já baixados ou
comprovantes impressos. **Se havia pagamento com resposta incerta, confira histórico
de pedidos/recebimentos antes de repetir:** a limpeza não envia tentativas antigas.

## Restauração e registro de remoções

O registro guarda IDs e datas de criação/remoção, sem contatos/textos pessoais.
Remoções anteriores são importadas durante a atualização. IDs e datas de criação
de loja/cliente precisam coincidir, evitando confundir cadastros diferentes.

A restauração Windows combina o registro do banco atual com o do backup e reaplica
a limpeza **no banco temporário, antes da promoção transacional**. Se a preparação
falhar, o banco atual permanece. Sessões são revogadas e políticas automáticas ficam
desativadas para revisão. Remoções de cadastros ainda ausentes no backup continuam
registradas, protegendo restaurações seguintes.

No Linux, use `python3 scripts/restore-compose.py arquivo.dump previo.dump` a partir
do repositório com a imagem atualizada. O app para; o script exporta o registro,
gera `previo.dump` e `previo.dump.privacy.json`, migra/limpa um staging e promove em
transação. O caminho prévio deve ser novo. O procedimento anterior com `pg_restore`
direto é **superseded**: ele ignora essa reconciliação.

Backups/exportações anteriores não são modificados nem têm retenção automática.
Proteja também o registro de remoções e o backup recente: **um backup antigo sozinho
em outra máquina não contém solicitações futuras**. Não descarte o banco/registro
recente antes da restauração. Em recuperação sem a base recente, transporte as
remoções por procedimento de manutenção antes de retomar o atendimento.

## Política de retenção por loja

Em **ADM → Clientes → Ferramentas LGPD → Política de retenção**, informe dias sem
atividade, simule o impacto e confirme explicitamente para ativar. O padrão é
**desativado**, sem prazo sugerido ou fiscal presumido. Defina o prazo segundo a
política da sua operação. Desmarcar a ativação interrompe novas limpezas; não recupera
textos removidos. Outra edição concorrente exige reabrir a política.

A rotina verifica a cada minuto enquanto o aplicativo está ligado, em lotes de até
50 clientes por loja. Usa a mesma limpeza manual, transação e registro de remoções.
Falhas desfazem o lote e ele é reavaliado no próximo ciclo. Atividade inclui
alterações do cadastro, consentimentos, pedidos/transições, recebimentos/devoluções,
pagamentos e fechamento de comandas vinculadas. Pedidos em andamento, pendências
financeiras ou comandas abertas excluem o cliente da limpeza automática.
Pedidos legados começam seu relógio de transições na migração, de forma conservadora.

São preservados centavos, métodos, IDs, datas, composição e hashes de retries.
Campos gerais sem vínculo, arquivos e prazos de guarda dos registros financeiros
exigem política própria; esta rotina não apaga lançamentos financeiros.

API: `GET /api/customer-privacy-review?customer_id=N` retorna contagens e uma versão.
Envie a versão no header `X-Privacy-Review` ao remover. Versão divergente retorna 409.
As rotas antigas de DELETE continuam compatíveis sem esse header; a UI usa revisão.
Somente administradores/gerentes com gestão de usuários podem revisar e remover.

API da política: `GET /api/privacy-policy` e `GET /api/privacy-policy?days=N`
retornam versão, ativação, prazo e quantidade elegível. `POST` exige `version`,
`enabled`, `inactive_days` e `confirm:true` para ativar; versão divergente retorna
409. `GET /api/privacy-state` retorna somente a revisão, para operadores da loja.
O registro de remoções integra os backups recentes e não contém contatos.

[Evidências e limites](../project-memory/privacy-completion-stage.md).
