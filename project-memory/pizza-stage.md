# Pizza com múltiplos sabores · etapa 0.12 · 2026-10-12

Skills: Backend Architect e UX Architect. Próxima entrega do foco catálogo restaurante; implementação em validação, não concluída.

- Configuração por produto, 1–4 sabores por pizza, até 30 sabores cadastrados; preços de pizza inteira no tamanho base e ajuste comum do tamanho. Venda apenas por unidade.
- Política explícita: maior preço ou média ponderada pelas partes (1–8 partes inteiras no total). Centavos inteiros; arredondamento ao centavo mais próximo, empate para cima, antes de tamanho/adicionais.
- Preço calculado no servidor, sabores ativos da mesma configuração, versões para carrinhos/edições concorrentes; snapshot preserva nomes, partes, preço e política. PDV, combo e rodada de comanda usam o mesmo caminho transacional.
- Estoque de produto/tamanho/adicional em pizzas inteiras; não modela ingredientes ou estoque por fração de sabor. Cancelamento recompõe stocks existentes.
- Schema v9 separado, preservando bytes/fingerprints v1–v8 e atualização de backups antigos.
- Jornada PostgreSQL focal PASS 18.08s: preço/snapshot/estoque/retries/isolamento/permissões/totais adulterados/cancelamento/comandas/backup/restauração/última pizza concorrente. Falhas iniciais nos fixtures de automação (payload de usuário, nomes de endpoints/ação) superseded pelas correções e repetição.
- Gate local: suíte completa PostgreSQL PASS 240.080s, incluindo upgrade e restauração v1–v8. Jornada focal final ampliada PASS 21.39s, incluindo pizza em combo e unidade alterada. Vet e sintaxe PASS; govulncheck zero alcançáveis e uma em módulo requerido sem chamadas; npm audit zero.
- Chromium fresco `pizza-20261012-c`: núcleo/setup/backup, ADM, final e pizza PASS; zero pageerrors. UI da 0.12 validou edição de sabores, média/partes, variante/adicional, mobile/escuro, reload, recebimento e comprovante; pizza em combo também verificada. Capturas inspecionadas em `docs/evidence/pizza-stage/`; regressões redirecionadas para essa etapa, preservando evidências antigas. Falhas iniciais de automação (retorno ao seletor após reload, formato da resposta de detalhes e reutilização de nomes únicos em fixtures) superseded pela repetição em base nova; suíte final exige ser executada antes de criar produtos extras.
- CI/release pendentes. Próximos itens do foco: adicionais compartilhados, horários por loja/categoria e privacidade/retenção conforme política definida.
