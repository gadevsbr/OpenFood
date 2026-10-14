# Horários de venda · etapa 0.14 · 2026-10-13

Skills: Backend Architect e UX Architect. Implementada e validada localmente; publicação pendente.

- Horários semanais por loja e categoria, múltiplas faixas, meia-noite e fuso da loja.
  Restrições se somam: loja, categorias superiores e janela diária existente do produto.
- Horários semanais incluem abertura e excluem fechamento. Fim 1440 representa
  meia-noite; 0–1440 cobre o dia inteiro. Regra diária antiga mantém limites inclusivos.
- Sem limite por padrão; com limite sem faixas fecha a semana inteira.
  Categorias inativas/cíclicas falham fechadas. Mudança exige confirmação da loja/produtos.
- Schema v10 separado preserva fingerprints v1–v9. Padrão America/Sao_Paulo mantém
  horários anteriores sem ativar limites semanais na atualização. Categoria inativa passa a restringir produtos conforme a decisão registrada.
- Servidor confere disponibilidade na venda; PDV usa available_now e a atualização
  periódica existente. Edição e vendas compartilham lock da loja; versão/CAS impede sobrescrita.
- PostgreSQL focal PASS 26.15s: limites, UTC/local, meia-noite, DST repetido,
  categorias superiores/ciclo, janela antiga, retries, estoque, isolamento, permissões e restauração.
- PostgreSQL completo PASS 383.943s, incluindo upgrades/restauração v1–v9; focal ampliado PASS 34.46s com combo, checkout fechado, comanda, concorrência de fechamento e cancelamento. Chromium núcleo/ADM/final/horários PASS, zero pageerrors, desktop/mobile/escuro inspecionados. Primeira falha de sincronização do diálogo e fixture sem caixa aberto: superseded pelas correções e repetição.
- Vet/sintaxe/audit frontend PASS; govulncheck zero alcançáveis/importadas e uma em módulo requerido sem chamada. CI/release/checksums pendentes.
  Clientes/privacidade seguem próximos; periféricos, ambiente limpo e fiscal continuam abertos.

Revisão final: API rejeita novo ciclo de categoria, preservando a resposta anterior para categoria como própria mãe. PostgreSQL focal final PASS 34.46s; repetição Chromium em base fresca schedule-20261013-b PASS. Publicação ainda pendente.
