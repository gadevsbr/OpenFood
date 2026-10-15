# Privacidade · etapa 0.16 · 2026-10-14

Autorizados os três pontos em conjunto. Skills Privacy Engineer, Backend Architect,
UX Architect e UI Finish-Gate Reviewer. Em implementação; sem gate de pronto.

- Registro de remoções sem contatos/textos; identidade composta por IDs e datas de
  criação da loja/cliente, evitando apagar cadastro diferente que reutilize número.
  Migração v11 separada e backfill das remoções anteriores. Fingerprints preservados.
- Restauração suportada combina o registro atual com o backup e limpa o staging
  antes da promoção transacional. Políticas restauradas ficam desativadas; sessões
  revogadas. Compose ganha script de restauração com backup prévio e staging.
- PDV verifica revisão antes das chamadas e periodicamente/foco/reconexão. Mudança
  descarta rascunhos e carrinhos em espera e recarrega dados/telas. Aviso orienta
  conferir histórico antes de repetir pagamento com resposta desconhecida.
  Sem conexão, conteúdo fica coberto até a verificação. Revisão não contém PII.
- Política por loja, desativada inicialmente: dias de inatividade definidos pelo
  gestor, simulação, confirmação explícita e versão/CAS. Worker por minuto em
  lotes de 50, mesmos locks da venda. Operações abertas/pendentes são excluídas.
  Não define prazo fiscal; financeiro preservado. Backups/exportações físicos e
  campos sem vínculo continuam exigindo tratamento próprio.
- Limites anteriores de ausência de reaplicação, coordenação de abas e política
  configurável: superseded somente após os testes da etapa. APIs/GUI antigas
  precisam ser reabertas ao atualizar. Um backup antigo sozinho em outra máquina
  não contém solicitações futuras: é necessário transportar o registro recente.

Primeiro focal detectou falta de migração v11 no bootstrap; corrigido. Repetição
inicial com remoção e backup antigo PASS 32.561s. Cobertura ampliada em execução.


Focal ampliado PostgreSQL PASS 54.244s: confirmação/versão/permissões, candidatos,
pedidos/comandas abertas, atividade recente de transição/recebimento, rollback de
falha, dois workers concorrentes/idempotência, colisão de IDs/datas e restauração
antiga repetida. Retenção restaurada desativada. Chromium núcleo/ADM/final/continuidade
PASS em base c sintética, zero pageerrors. Aba paralela e sessão offline/reconectada
descartaram rascunhos/espera/tentativa. Retenção desktop/mobile/escuro inspecionada:
CTA fixo no rodapé do diálogo e estados de erro/confirmação preservados.
Testes antigos de offline permitindo leitura/interação: superseded pela cobertura
modal e reconexão verificada. QA b com fingerprint v11 intermediário recusado como
esperado; repetição limpa c usada. Suíte completa intermediária PASS 460.497s;
suíte definitiva/CI/publicação/hashes ainda pendentes.


Revisão final adicionou à auditoria os dias/ativação/versão aprovados; falhas da
rotina registram somente código operacional, sem dados pessoais. Focal repetido
em execução; governação não define prazo legal. Vet/sintaxe/audit frontend PASS;
govulncheck zero alcançáveis/importadas e uma em módulo requerido sem chamadas.
Contrato visual: gestor revisa prazo/impacto; ação Salvar fixa, contagem sem nomes,
formulário com confirmação, temas do produto. Padrões de referência já registrados
em ui-contract.md/restaurant-ui-stage.md reaproveitados. Finish gate local PASS
nas capturas finais desktop/mobile/escuro, incluindo erro e modal offline.

Gate local final: PostgreSQL completo PASS 423.924s (v1–v10→v11); focal revisado PASS 59.444s, após auditoria da política e revisão inicial sem descarte indevido. Chromium vinculado/JSON/stale review PASS. Gates locais substituem pendências anteriores (superseded); CI/Compose/ciclo instalado/publicação/hashes pendentes.

Upgrade focal legado v10→v11 PASS 20.352s com remoção legada importada e texto limpo após restore. Regressão Chromium combos/clientes PASS após adaptar reload e seletor ao cliente específico. Pendências locais anteriores superseded; distribuição continua pendente.
