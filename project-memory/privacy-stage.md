# Revisão de privacidade · etapa 0.15 · 2026-10-14

Skills: Backend Architect e Privacy Engineer. Implementada e validada localmente; CI/publicação pendentes.

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
- PostgreSQL completo PASS 412.212s, com upgrades/restauração v1–v9; focal final PASS 25.12s. Verifica rollback pelo JSON inteiro, conflitos, limpeza e valores financeiros, dados de outros clientes, retries, refund posterior, isolamento/permissões e backup novo.
- Chromium núcleo/ADM/final/privacidade PASS em base sintética isolada; desktop/mobile/escuro e download real JSON, zero pageerrors. Contagens compactadas para manter clareza no celular; capturas finais inspecionadas.
- Vet/sintaxe/diff/audit frontend PASS; govulncheck zero alcançáveis/importadas e uma em módulo requerido sem chamadas. CI/publicação/checksums pendentes.

Expectativa incorreta de total da segunda rodada na fixture: superseded pelo preço da rodada, corrigido e repetido. Promessa antiga de remoção/anonimização abrangente: superseded pelo escopo explícito de registros vinculados e limites em docs/privacy.md.
