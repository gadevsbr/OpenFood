# Plano e aceite
1. Registrar arquitetura e limites: diretório inicialmente vazio.
2. Implementar núcleo compartilhado Go/PostgreSQL: setup, sessão, catálogo, pedidos, migrações e fila durável (concluído na v0.1.0-alpha.1).
3. Implementar supervisor Windows: banco empacotado, instância única, tray, navegador, dados por usuário e encerramento (concluído na v0.1.0-alpha.1).
4. Backup/restauração, diagnóstico, instalador, Compose e pipeline de release (concluído na v0.1.0-alpha.1).
5. Execução do roadmap completo de evolução de produto: detalhado em [roadmap.md](roadmap.md) (Fases 2 a 9).

Aceite: primeira execução sem ferramentas, dados preservados, API autenticada, transações monetárias em centavos, migrações atômicas, recuperação de fila, backup restaurável. Windows limpo e Linux devem ter evidência separada de compilação.
Integrações externas não serão simuladas como conectadas; credenciais e homologações permanecem necessárias.

