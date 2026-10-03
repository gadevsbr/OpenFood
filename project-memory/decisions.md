# Decisões
- 2026-10-02: PostgreSQL 17 nas duas distribuições; Windows gerencia binários locais. SQLite não adotado. Não há decisão anterior substituída.
- Filas PostgreSQL transacionais, com leases e SKIP LOCKED; Redis não obrigatório.
- Código de negócio único. Windows usa navegador padrão e bandeja; fechar navegador mantém o processo.
- Instalação por usuário, dados em LocalAppData/OpenFood separados do programa em LocalAppData/Programs/OpenFood.
- Loopback obrigatório nesta etapa. Acesso LAN ainda não implementado; nenhum bind público implícito.
- Atualizações do aplicativo mantêm PostgreSQL major 17; upgrades de major exigem procedimento separado.
- Go 1.26.5 e dependências iniciais: superseded pela varredura govulncheck. Usar Go 1.26.8, pgx 5.9.2 e versões corrigidas de x/text, crypto e sys, fixadas em go.mod/go.sum.
- PostgreSQL Windows mostrou falha de bootstrap quando binários ficam em caminho acentuado (bug upstream 16926). Solução inicial apenas 8.3: superseded por 8.3 + alias DOS por sessão quando indisponível. Arquivos e dados permanecem no local escolhido; mapeamento temporário usa letra livre e é removido após shutdown. Teste força essa alternativa sem mudar configurações do volume.
