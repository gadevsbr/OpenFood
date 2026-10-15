# Roadmap OpenFood — Do MVP Local ao Produto Completo

Este roadmap consolida todos os requisitos estabelecidos na especificação original do OpenFood, divididos em fases evolutivas com entregas e critérios de aceite verificáveis por testes automatizados e evidências reais.

## Foco atual — catálogo e clientes/privacidade
Autorizado pelo usuário em 2026-10-11. Responsável: implementação OpenFood.
Horizonte: próximas entregas incrementais, sem prazo de calendário prometido.
Skill de decisão: Product Manager (Agency Agents).

1. **Catálogo restaurante:** integrar combos ao PDV e às mesas, com composição,
   preço no servidor, estoque dos componentes e snapshot na cozinha/comprovante.
   Depois, sabores de pizza e regras explícitas de preço; edição de adicionais
   compartilhados. Horários já existem em America/Sao_Paulo: revisar UX e cobertura,
   completar horários por loja/categorias sem recriar o mecanismo anterior.
2. **Clientes e privacidade:** revisar cadastro, consentimentos, revogação,
   exportação e anonimização já existentes. Completar vínculo no atendimento,
   histórico por loja, evidência/versionamento de consentimentos e política de
   retenção explícita. Não presumir prazo fiscal nem apagar dados automaticamente
   sem política definida; preservar registros financeiros e eliminar identificadores
   pessoais conforme o escopo aprovado.

Aceite por entrega: jornada PostgreSQL real, isolamento/permissões, concorrência,
backup/restauração e Chromium desktop/mobile; release experimental com checksums.
Metas verificáveis: preço/estoque corretos sem duplicação em retries; nenhuma
exposição entre lojas; revogação efetiva e exportação/anonimização consistentes.
Peso/mercados continua no roadmap, após o escopo prioritário de restaurante.
Integrações Pix, WhatsApp, iFood e novas frentes de entrega ficam para depois.
Recomendação anterior de avançar diretamente para Pix após combos: **superseded**
pelo foco explícito nos itens 1 e 2. Nenhuma funcionalidade nova declarada pronta.

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
2026-10-13: etapa 0.14 horários semanais por loja/categoria e fuso validada e publicada, PostgreSQL/Chromium e CI 38497740815 PASS, hashes conferidos. Gate de distribuição superseded. [Escopo](schedule-stage.md). Próxima frente: clientes/privacidade. Fase 3 continua aberta para peso/mercados e critérios restantes.
2026-10-12: etapa 0.13 adicionais compartilhados validada e publicada, PostgreSQL/Chromium e CI 38324219895 PASS, hashes conferidos. Pendência de distribuição superseded. [Escopo](addons-stage.md). Próxima entrega: horários por loja/categoria; depois revisão de clientes/privacidade. Fase 3 aberta.
2026-10-12: etapa 0.12 integra sabores de pizza e preço maior/média ponderada no servidor; PostgreSQL/Chromium e CI 38320810285 Windows/Linux/publicação PASS, release experimental com checksums conferidos. Pendência de distribuição superseded. [Escopo e limites](pizza-stage.md). Adicionais compartilhados e horários por loja/categoria continuam próximos; Fase 3 não concluída.
2026-10-11: etapa 0.10 valida variantes/tamanhos, adicionais, observações e fotos no PDV/mesas. [Evidências e limites](menu-stage.md). Etapa 0.11 valida combos operacionais; sabores de pizza e adicionais compartilhados continuam pendentes. [Escopo](combo-customer-stage.md). Critérios completos da fase ainda abertos.

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
2026-10-14, etapa 0.16: os três pontos autorizados foram implementados: registro/reaplicação
de remoções no staging da restauração; descarte coordenado de rascunhos/telas do PDV;
política de retenção por loja, simulação e ativação explícita, inicialmente desativada.
PostgreSQL completo 423.924s, focal 59.444s e Chromium PASS; distribuição em andamento. [Etapa](privacy-completion-stage.md).
Pendências anteriores desses três mecanismos: superseded no escopo testado, sem
certificação legal ou apagar arquivos/campos sem vínculo. Fase 4 mantém esses limites explícitos.

2026-10-14: etapa 0.15 amplia limpeza de textos vinculados e revisão de impacto; PostgreSQL/Chromium e CI 38668909597 PASS, release experimental publicada e hashes conferidos; gate de distribuição superseded. [Escopo](privacy-stage.md). Retenção automática, campos sem vínculo e registro/reaplicação de remoções após backup antigo continuam abertos.
2026-10-11: etapa 0.11 valida vínculo no PDV/rodadas, histórico por loja, texto/versionamento de consentimentos, exportação e remoção manual de identificação. Retenção automática, campos livres e remoções após restaurar backups seguem abertos. [Evidências e limites](combo-customer-stage.md).
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
