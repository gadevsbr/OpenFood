# Plano e aceite
1. Registrar arquitetura e limites: diretório inicialmente vazio.
2. Implementar núcleo compartilhado Go/PostgreSQL: setup, sessão, catálogo, pedidos, migrações e fila durável.
3. Implementar supervisor Windows: banco empacotado, instância única, tray, navegador, dados por usuário e encerramento.
4. Backup/restauração, diagnóstico, instalador, Compose e pipeline de release.
5. Executar testes reais disponíveis e registrar matriz de ambientes.

Aceite: primeira execução sem ferramentas, dados preservados, API autenticada, transações monetárias em centavos, migrações atômicas, recuperação de fila, backup restaurável. Windows limpo e Linux devem ter evidência separada de compilação.
Integrações externas não serão simuladas como conectadas; credenciais e homologações permanecem necessárias.
