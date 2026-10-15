# Continuidade da privacidade · etapa 0.16 · 2026-10-14

Três pontos autorizados implementados, testados e publicados no escopo abaixo.
Skills Privacy Engineer, Backend Architect, UX Architect e UI Finish-Gate Reviewer.

- Registro transacional de remoções com IDs/datas de criação, sem contatos/textos;
  importa remoções legadas. Schema v11 separado preserva fingerprints v1–v10.
- Restauração Windows e script Compose combinam registros atuais/backup e limpam
  staging antes da promoção transacional. Backup prévio, sessões revogadas e
  políticas automáticas restauradas desativadas para revisão. Pedidos ausentes no
  snapshot continuam registrados; ID numérico reutilizado não identifica outra pessoa.
- Abas do PDV verificam revisão antes das chamadas, por intervalo/foco/reconexão;
  mudança descarta rascunhos/espera/tentativas locais e recarrega dados/formulários.
  Modal offline bloqueia leitura/interação até a verificação. Atualização exige
  reabrir abas antigas; navegador suspenso executa limpeza quando retoma/conecta.
  Resposta de pagamento incerta exige conferir histórico antes de repetir.
- Política por loja inicialmente desativada, dias definidos pelo gestor, simulação,
  confirmação/versionamento e auditoria dos valores aprovados. Worker por minuto,
  lotes de 50, mesmos locks da venda; operações/comandas abertas e saldos pendentes
  excluídos. Atividade recente de cadastro, consentimento, pedido/transição e
  recebimento/pagamento/fechamento reinicia o relógio. Financeiro preservado.
- Não define prazo fiscal/anonimato; arquivos, impressos e textos sem vínculo
  exigem tratamento próprio. Backup antigo isolado não contém solicitações futuras:
  preserve banco/registro recente ao recuperar em outra máquina. Veja docs/privacy.md.

Evidências: PostgreSQL completo PASS 423.924s, upgrades/restaurações v1–v10→v11;
focal final PASS 59.444s após auditoria/revisão inicial; legado v10 focal PASS
20.352s, incluindo backfill e limpeza no restore. Rollback, concorrência, retries,
atividade recente, permissões, colisão de identidade e financeiro preservado.
Chromium núcleo/ADM/final/revisão/continuidade/combos PASS, zero pageerrors; sessão
offline/reconectada e abas paralelas limparam rascunhos. Capturas desktop/mobile/
escuro inspecionadas, CTA Salvar fixo e confirmação/erros preservados. Vet/sintaxe/
audit frontend PASS; govulncheck zero alcançáveis/importadas, uma em módulo sem chamadas.

Falhas iniciais de bootstrap sem v11, expectativas antigas de offline e seletor de
consentimento sem cliente: corrigidas, repetições PASS **superseded**. QA com schema
v11 intermediário recusado; base final c sintética usada, sem dados de trabalho.
Pendências dos três mecanismos na 0.15 e gates intermediários: **superseded** pelo
escopo/evidências desta etapa. Fase 4 mantém limites de campos sem vínculo/guarda.

Distribuição concluída: [CI 38672838655](https://github.com/gadevsbr/OpenFood/actions/runs/38672838655) Linux/Windows/publicação PASS,
incluindo Compose com backup anterior à remoção, PostgreSQL real e ciclo instalado.
[Release experimental 0.16](https://github.com/gadevsbr/OpenFood/releases/tag/v0.16.0-alpha.1) com seis assets; quatro artefatos baixados,
SHA-256 iguais aos manifests e digests GitHub. Gates de distribuição superseded.
Fonte f4f1ddb; encerramento documental em main. QA encerrado. Ambiente
limpo, periféricos, fiscal, assinatura e reboot físico continuam sem certificação.
