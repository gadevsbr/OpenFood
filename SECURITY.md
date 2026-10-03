# Segurança
Não publique credenciais, dumps ou dados pessoais em issues. Reporte vulnerabilidades por canal privado no GitHub quando disponível.

Este alpha limita HTTP a loopback, valida Host/Origin, exige header anti-CSRF em mutações, usa cookies HttpOnly/SameSite Strict, bcrypt e sessões revogáveis. Não habilita conectores externos. Administrador da instalação tem acesso a todos os recursos da própria loja; papéis granulares ainda pendentes.

Banco/configuração local têm ACL de usuário e SYSTEM; senha do banco é gerada, não registrada em logs. Dump não é criptografado. Segredos externos criptografados, recuperação de senha, assinatura de distribuição, hardening completo e auditoria independente são pendentes. Não há afirmação automática de conformidade legal.
