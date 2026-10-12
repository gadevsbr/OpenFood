# Adicionais compartilhados · etapa 0.13 · 2026-10-12

Skills: Backend Architect e UX Architect. Implementada, validada e publicada no escopo testado; Fase 3 permanece aberta.

- Editor conjunto de grupo/opções/vínculos, com lista e confirmação dos produtos
  afetados, busca, temas e acesso pelo cadastro de produto. Rascunho do produto preservado.
- Transação sob lock da loja, versões de configuração de todos os produtos afetados,
  conflito de edição e comparação do estoque antes da gravação. Estoque único por opção.
- Endpoints legados de criação/vínculo também usam o lock, auditam alterações,
  conferem loja e atualizam versões. Dados e snapshots preservados, sem novo DDL: schema v9.
- PostgreSQL focal inicial PASS 13.80s: preços, estoque compartilhado, retries,
  concorrência, conflito de cadastro/estoque, propagação, desvínculo, permissões e isolamento.
- Suíte completa PostgreSQL PASS 263.342s; jornada focal final PASS 28.92s com cancelamento de opção inativa e backup/restauração. Chromium limpo núcleo/ADM/final/adicionais PASS, zero pageerrors; desktop/mobile/escuro inspecionados. Vet/sintaxe e audit frontend PASS. CI/publicação/checksums pendentes.
- Testes antigos de CTA superseded pelo diálogo conjunto. Fixtures de UI corrigidos
  para resposta de criação contendo apenas ID; repetição limpa PASS substitui as falhas iniciais (superseded).
- Horários por loja/categoria e revisão de clientes/privacidade continuam seguintes.
  Hardware, ambiente limpo, provedores externos e fiscal seguem sem certificação.
- Govulncheck v1.8.0: zero vulnerabilidades alcançáveis/importadas; uma em módulo requerido sem chamadas. Última repetição Chromium adicionais PASS após ajustar captura/foco para remover o balão nativo de validação. Evidências somente sintéticas.

Distribuição concluída: CI [38324219895](https://github.com/gadevsbr/OpenFood/actions/runs/38324219895) Linux/Windows/publicação PASS, com Compose, PostgreSQL real e ciclo instalado. [Release experimental v0.13.0-alpha.1](https://github.com/gadevsbr/OpenFood/releases/tag/v0.13.0-alpha.1): seis assets; quatro artefatos baixados e hashes iguais aos manifests e digests GitHub. Gates CI/publicação/checksums antes pendentes: **superseded** pela evidência. Fonte 6c84bd4; encerramento documental em main. Evidências `docs/evidence/addons-stage/release-*.json`. Bases QA encerradas; dados de trabalho não utilizados.
