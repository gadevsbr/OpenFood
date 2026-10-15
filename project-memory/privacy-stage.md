# Revisão de privacidade · etapa 0.15 · 2026-10-14

Skills: Backend Architect e Privacy Engineer. Implementada, validada e publicada no escopo testado. Fase 4 continua aberta.

- Revisão por cliente/loja mostra contagens dos registros afetados, aviso de textos
  financeiros e comandas compartilhadas, confirmação explícita e conflito de revisão.
- Limpeza ampliada: texto do consentimento, motivos de recebimentos/devoluções,
  descontos e rótulos de comandas vinculadas, além dos campos já limpos na 0.11.
- Campos financeiros (centavos, pagamentos, descontos), IDs, datas, composição,
  hashes de retries e registros de outros clientes preservados. Comanda compartilhada
  tem rótulo substituído; itens de outros clientes permanecem.
- Remoção usa lock da loja e do caixa. Recebimentos posteriores de pedidos vinculados
  a cadastro removido guardam marcador em vez do texto informado, preservando retries.
- Exportação inclui notas de recebimentos, descontos e comandas vinculadas; remove
  promessa de "todos os dados"/anonimato/conformidade fiscal da UI. Schema v10 sem DDL novo.
- Retenção automática, campos sem vínculo, textos gerais de catálogo, backups e
  exportações antigos continuam fora desta limpeza. Sem prazo fiscal inventado.
- Mapa adicional verificado em web-pos.js: sessionStorage guarda itens/observações,
  cliente do carrinho e motivo de desconto por operador/loja. Rascunhos/carrinhos
  em espera, telas abertas e comprovantes já impressos não são limpos por esta
  transação PostgreSQL. Coordenação entre sessões de navegador continua pendente.
- PostgreSQL completo PASS 412.212s, com upgrades/restauração v1–v9; focal final PASS 25.12s. Verifica rollback pelo JSON inteiro, conflitos, limpeza e valores financeiros, dados de outros clientes, retries, refund posterior, isolamento/permissões e backup novo.
- Chromium núcleo/ADM/final/privacidade PASS em base sintética isolada; desktop/mobile/escuro e download real JSON, zero pageerrors. Contagens compactadas para manter clareza no celular; capturas finais inspecionadas.
- Vet/sintaxe/diff/audit frontend PASS; govulncheck zero alcançáveis/importadas e uma em módulo requerido sem chamadas. CI/publicação/checksums pendentes.

Expectativa incorreta de total da segunda rodada na fixture: superseded pelo preço da rodada, corrigido e repetido. Promessa antiga de remoção/anonimização abrangente: superseded pelo escopo explícito de registros vinculados e limites em docs/privacy.md.

Distribuição concluída: CI [38668909597](https://github.com/gadevsbr/OpenFood/actions/runs/38668909597) Linux/Windows/publicação PASS, com PostgreSQL real, upgrades/restauração v1–v9, Compose e ciclo instalado. [Release experimental v0.15.0-alpha.1](https://github.com/gadevsbr/OpenFood/releases/tag/v0.15.0-alpha.1): seis assets; quatro artefatos baixados e hashes iguais aos manifests e digests GitHub. Gates CI/publicação/checksums antes pendentes: **superseded** pela evidência. Fonte 7aeb05b; encerramento documental em main. Evidências `docs/evidence/privacy-stage/release-*.json`. Base QA encerrada; dados de trabalho não utilizados. Próximo: registro/reaplicação de remoções após restaurar backup antigo; retenção/campos sem vínculo ainda abertos.
