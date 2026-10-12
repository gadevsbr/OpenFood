# Segurança
Não publique credenciais, dumps ou dados pessoais em issues. Reporte vulnerabilidades por canal privado no GitHub quando disponível.

Este alpha limita HTTP a loopback, valida Host/Origin, exige header anti-CSRF em mutações, usa cookies HttpOnly/SameSite Strict, bcrypt e sessões revogáveis. Não habilita conectores externos. Há permissões por papel, isolamento por organização/loja e revogação de sessões ao desativar usuários ou alterar papéis. Consulte o escopo e as evidências em project-memory; esses testes não substituem auditoria independente.

Banco/configuração local têm ACL de usuário e SYSTEM; senha do banco é gerada, não registrada em logs. Dump não é criptografado. Segredos externos criptografados, recuperação de senha, assinatura de distribuição, hardening completo e auditoria independente são pendentes. Não há afirmação automática de conformidade legal.
