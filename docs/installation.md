# Instalação e operação
## Windows 10/11 x64
Execute OpenFood-0.1.0-alpha.1-windows-x64-setup.exe. Instalação por usuário, sem Go, Node, Docker ou PostgreSQL instalados manualmente. Abra OpenFood pelo menu Iniciar; o supervisor inicializa o banco local e abre o navegador somente após prontidão. Crie loja, e-mail e senha de pelo menos 12 caracteres.

Programa: `%LOCALAPPDATA%\Programs\OpenFood`. Dados: `%LOCALAPPDATA%\OpenFood`, com ACL para usuário atual e SYSTEM. Não compartilhe config.json: contém senha gerada do banco e token temporário de setup, inutilizável após criar o administrador. Banco aceita apenas loopback com SCRAM.

Binários PostgreSQL em caminho acentuado usam alias 8.3 existente ou caminho DOS temporário por sessão, contornando o [bug upstream de bootstrap](https://www.postgresql.org/message-id/16926-e4ef345545d185ab%40postgresql.org). Não move arquivos nem exige administrador. Letras livres são verificadas antes do mapeamento e ele é removido no shutdown. Perfis Unicode fora da página de códigos ainda não foram validados; não há cobertura geral desses ambientes.

O painel prefere 18880 e escolhe outra porta se ocupada. A bandeja abre o endereço real. Segunda instância abre o painel existente. Iniciar com Windows é opt-in na bandeja, por conta de usuário; não roda antes do login. Fechar o navegador mantém o processamento. A opção de encerrar automaticamente ao fechar o navegador ainda não está implementada.

Encerrar pela bandeja drena HTTP, para worker e encerra PostgreSQL. O evento de encerramento da sessão Windows é encaminhado pela biblioteca da bandeja; reboot físico permanece pendente. Após queda, PostgreSQL recupera WAL e jobs retomam leases expiradas. Se banco não inicia, não apague postmaster.pid; verifique processos, espaço em disco e operations.log.

Atualização: encerre pela bandeja, rode novo instalador no mesmo diretório. Preparação bloqueia atualização com app em execução, gera pg_dump e verifica leitura do arquivo. O programa novo verifica checksum de migração e transação de esquema. Não há atualização automática, assinatura Authenticode nem certificação de migrações futuras; ensaio prévio de cada versão em cópia restaurada é obrigatório antes de release estável. PostgreSQL major 17 não pode ser substituído por major 18 diretamente.

Desinstale por Configurações do Windows. Dados são preservados por padrão; backups permanecem na pasta de dados. Instalação/reinstalação devem usar a mesma conta Windows.

## Linux
Instale Docker Engine e Compose conforme documentação oficial. Copie `.env.example` para `.env`; gere dois valores hexadecimais aleatórios de 32 bytes para POSTGRES_PASSWORD e SETUP_TOKEN, sem reusar senhas. Não comite `.env`.

```sh
docker compose up --build -d
curl http://127.0.0.1:18880/readyz
```
Abra http://127.0.0.1:18880. O setup Linux solicita o SETUP_TOKEN definido no servidor. Banco não publica portas. Volume `database` persiste dados. Não use `docker compose down -v` para atualizar.

```sh
docker compose -f compose.yaml -f compose.dev.yaml up --build
```

Backup Linux, no host com Compose:
```sh
umask 077
docker compose exec -T db pg_dump -U openfood -d openfood -Fc > openfood.dump
```
Restaure somente arquivos de origem confiável. Pare app, gere backup prévio, valide o dump em banco temporário, então:
```sh
docker compose stop app
cat openfood.dump | docker compose exec -T db pg_restore -U openfood -d openfood --clean --if-exists --single-transaction --no-owner --no-acl
docker compose exec -T db psql -U openfood -d openfood -c "DELETE FROM sessions; UPDATE jobs SET state='failed',last_error_code='restored_requires_review' WHERE state IN ('pending','running');"
docker compose start app
```
Backup pela interface Linux ainda indisponível; API retorna explicitamente 501. Atualize com backup, `git pull` e `docker compose up --build -d`. Rollback de binário só funciona quando esquema é compatível; caso contrário restaure backup anterior em volume separado e reconcilie efeitos.

## Backups e diagnóstico Windows
Painel → Saúde e manutenção → Gerar backup. O dump é baixado e uma cópia permanece em `backups`. Inclui dados pessoais e hashes de senha: não é criptografado, proteja o arquivo e o disco (por exemplo, BitLocker). Este alpha não contém sessões de WhatsApp nem credenciais externas.

Restaurar: selecione dump próprio, confirme RESTAURAR. Validação ocorre em banco temporário, backup prévio é obrigatório e substituição usa transação única. Sessões são revogadas; tarefas pendentes ficam em revisão, sem reenvio externo automático. Nunca restaure dumps recebidos de terceiros: pg_restore executa SQL do arquivo.

Diagnóstico exporta apenas códigos operacionais e estado local, sem senha, token, dados pessoais, banco ou logs brutos. Logs de PostgreSQL não são incluídos. Rotação/retenção de arquivos ainda pendente.

## Conectividade
Loopback é o único modo suportado neste alpha. Não altere portas para `0.0.0.0`. LAN autenticada e assistente de túnel HTTPS ainda não implementados. Integrações públicas exigirão proxy/túnel com HTTPS e validação específica do provedor; localhost sozinho não recebe webhooks externos. PC desligado ou suspenso não opera. Não há servidor central obrigatório.
