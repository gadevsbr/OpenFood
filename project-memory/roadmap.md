# Roadmap OpenFood — Do MVP Local ao Produto Completo

Este roadmap consolida todos os requisitos estabelecidos na especificação original do OpenFood, divididos em fases evolutivas com entregas e critérios de aceite verificáveis por testes automatizados e evidências reais.

---

## Fase 1: Fundação do Núcleo e Distribuição Local (Concluída — v0.1.0-alpha.1)
- [x] Monólito Go compartilhado com PostgreSQL 17 (empacotado no Windows x64 e Docker Compose no Linux).
- [x] Setup inicial protegido com token de uso único e sem senha padrão.
- [x] Sessões com hash criptográfico, proteção CSRF e restrição rígida de Host/Origin para loopback.
- [x] Catálogo básico e pedidos manuais com valores monetários em centavos BRL (`bigint`) e snapshot de preços.
- [x] Controle transacional de estoque (`SELECT FOR UPDATE`) impedindo venda concorrente da última unidade.
- [x] Máquina de estados de pedidos comerciais e liberação atômica de estoque em cancelamentos.
- [x] Fila durável local no PostgreSQL (`jobs`) com `SKIP LOCKED` e leases contra travamentos.
- [x] Supervisor Windows com ícone na bandeja (tray), single-instance e rotinas seguras de backup/restore `.dump`.
- [x] CI/CD multi-plataforma e publicação de release com checksums SHA-256.

---

## Fase 2: Organizações, Lojas e Controle de Acesso Granular (RBAC Multi-tenant) — **Concluída — v0.2.0-alpha.1**
- [x] **Múltiplas Organizações e Lojas**:
  - Suporte explícito a várias lojas operando na mesma instalação com isolamento estrito de dados.
  - Escopo de loja validado em nível de banco de dados (`WHERE store_id = $1`) em todas as queries e rotinas.
- [x] **Papéis Granulares e Permissões**:
  - Administrador da Instalação (`instance_admin`).
  - Administrador da Organização (`org_admin`).
  - Gerente de Loja (`store_manager`).
  - Atendimento (`attendant`).
  - Cozinha / Separação (`kitchen`).
  - Expedição (`dispatch`).
  - Consulta Financeira (`finance_viewer`).
- [x] **Critérios de Aceite**:
  - Testes automatizados tentando acesso cruzado entre lojas com rejeição `403 Forbidden`.
  - Sessões vinculadas a permissões ativas e revogação imediata via `token_version` em caso de desativação do usuário.

---

## Fase 3: Catálogo Avançado (Restaurantes e Mercados)
- [ ] **Categorias e Disponibilidade**:
  - Categorização hierárquica, horários de venda por loja e disponibilidade atômica (produto esgotado).
- [ ] **Modelagem para Restaurantes**:
  - Variantes, tamanhos, múltiplos sabores para pizzas/combos.
  - Grupos de adicionais e opções com limites de escolhas mínimas e máximas.
  - Política de preço determinística para combinações (maior valor ou média ponderada em centavos).
  - Remoção de ingredientes e observações customizadas do cliente.
- [ ] **Modelagem para Mercados (Grocery)**:
  - SKU, código de barras e produtos vendidos por unidade ou peso (quantidade decimal com precisão definida).
  - Preferência de substituição e reajuste durante a separação.
- [ ] **Critérios de Aceite**:
  - Cálculo de centavos 100% no servidor; rejeição de payloads com valores manipulados pelo cliente.

---

## Fase 4: Clientes, Consentimentos e Privacidade (LGPD-first)
- [ ] **Cadastro de Clientes e Consentimentos**:
  - Identificação de cliente por telefone/e-mail sem compartilhamento automático entre lojas independentes.
  - Registro formal de consentimentos: finalidade específica, origem, data/hora, versão do texto dos termos e evidência.
  - Interface para revogação de consentimento e minimização de dados coletados.
- [ ] **Políticas de Retenção e Anonimização**:
  - Rotinas para exportação de dados do titular e exclusão/anonimização respeitando prazos fiscais de guarda.

---

## Fase 5: Conectores de Pagamento Pix (Mercado Pago, Asaas, Efí)
- [ ] **Interface Abstrata de Pagamentos com Capacidades Explícitas**:
  - `CreateCharge`, `GetQRCode`, `CheckStatus`, `ProcessNotification`, `Refund`.
  - Tratamento individualizado por conector (regras específicas de Mercado Pago, Asaas e Efí).
- [ ] **Cobrança Vinculada e Idempotência**:
  - Registro de identificador interno, identificador externo da gateway, valor em centavos, expiração e idempotência.
  - Validação estrita da assinatura criptográfica de cada webhook.
  - Verificação ativa do pagamento via consulta à API (valor, moeda e conta recebedora) antes da confirmação.
- [ ] **Resolução Operacional de Exceções**:
  - Pagamento recebido após expiração, cancelamento ou quebra de estoque encaminhado para fila de resolução manual (sem confirmação tácita nem devolução silenciosa).
  - Devoluções com permissão, motivo, idempotência e trava de teto financeiro.
- [ ] **Critérios de Aceite**:
  - Testes de concorrência com webhook duplicado, fora de ordem, payload forjado e divergência de valores.

---

## Fase 6: WhatsApp & Chatbot de Pedidos (Oficial e Whatsmeow)
- [ ] **Interface Comum de Mensageria**:
  - Envio, recepção, correlação de threads, acompanhamento de conexão e deduplicação de mensagens.
- [ ] **Conector WhatsApp Oficial (Meta Cloud API)**:
  - Validação de webhooks oficiais (SHA-256 HMAC), templates aprovados e janela de atendimento de 24h.
- [ ] **Conector Whatsmeow (Go nativo)**:
  - Pareamento via QR Code no terminal/painel, persistência segura da sessão em banco com proteção de chaves.
  - Backoff exponencial de reconexão e controle de concorrência (apenas um worker ativo por sessão).
- [ ] **Máquina de Estados da Conversa (Chatbot Autônomo)**:
  - Fluxo completo em máquina de estados determinística (sem dependência obrigatória de IA paga):
    1. Identificação de loja/cliente -> 2. Menus/Opções -> 3. Seleção de itens -> 4. Adicionais/Opções -> 5. Entrega/Retirada -> 6. Cálculo de frete/total -> 7. Resumo e confirmação explícita -> 8. Geração do Pix -> 9. Confirmação do pagamento -> 10. Despacho para cozinha.
  - Comando de fuga para atendimento humano que pausa o bot imediatamente.
- [ ] **Módulo Opcional de IA para NLP**:
  - Interpretação semântica estrita para sugestão de itens (a IA nunca confirma pagamentos, não inventa produtos e não define preços).
  - Sanitização de entradas contra prompt injection.

---

## Fase 7: Módulo de Entrega e Retirada
- [ ] **Módulo Retirada (Takeaway)**:
  - Estimativa de preparo, código de retirada, validação de retirada e registro do atendente.
- [ ] **Módulo Entrega Própria**:
  - Cadastro de bairros e faixas de CEP com taxas configuráveis em centavos.
  - Pedido mínimo e despacho com confirmação pelo entregador (sem necessidade obrigatória de APIs pagas de mapas).

---

## Fase 8: Integração iFood (Food & Grocery)
- [ ] **Autenticação e Polling/Webhooks iFood**:
  - Fluxo OAuth2 com renovação atômica de tokens no backend.
  - Ingestão resiliente de eventos (polling ou SSE/webhook oficial) com persistência em inbox transacional antes do ACK.
- [ ] **Ciclo de Pedido iFood**:
  - Confirmação de pedido, despacho, cancelamentos acordados e sincronização de catálogo (respeitando acessos concedidos).
  - Tratamento adequado de telefones intermediados e respeito aos termos de proteção de dados.

---

## Fase 9: Conectividade, Túneis e Observabilidade Avançada
- [ ] **Guia e Assistente de Túnel para Webhooks**:
  - Assistente para configuração de túnel HTTPS compatível (Cloudflare Tunnel, etc.) para instalações locais sem IP público fixo.
- [ ] **Observabilidade Local**:
  - Métricas de latência da fila, atraso de workers, pagamentos pendentes de reconciliação e logs estruturados em JSON sem vazamento de segredos.
- [ ] **Rotinas de Reconciliação Pós-Restauração**:
  - Procedimento pós-restauração de backup para conciliar cobranças em aberto com os provedores antes de reemitir mensagens.
