# Matriz de aceite Windows
Execute em VM Windows 10 e 11 x64 limpa, conta sem ferramentas de desenvolvimento. Não usar sucesso deste host como substituto.

| Caso | Resultado neste host | Gate em VM limpa |
|---|---|---|
| Binário Windows | Compila | Executar artefato publicado |
| Paths com espaços/acentos | PostgreSQL real e UI passaram | Repetir no instalador |
| Setup/login/pedido manual | Teste de API e navegador passaram | Repetir com rede desligada |
| Backup/restauração | pg_dump/pg_restore e UI passaram | Repetir com artefato instalado |
| Segunda instância | Processo adicional saiu | Repetir e inspecionar painel |
| Porta ocupada | Porta alternativa passou | Repetir no Windows limpo |
| Reboot | Não executado | Autostart opcional, WAL/lease recuperados |
| Atualização com dados | Reinstalação mesma versão preservou e gerou backup | Duas versões diferentes; validar esquema |
| Crash | App e PostgreSQL immediate stop passaram | Queda de energia e reboot físico |
| Sem privilégios admin | Token sem elevação confirmado | Repetir com conta padrão nova |
| Desinstalação preserva dados | Teste passou | Repetir em VM |
| Tray | Processo iniciado, UI native não inspecionada | Menu, estado, encerrar e preferência |

Bloqueios adicionais: sem assinatura Authenticode, sem certificado, sem teste Linux/Compose local. Artefatos alpha não certificam todos os requisitos.
