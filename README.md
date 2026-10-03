# OpenFood
Software livre para operação local de lojas, sem servidor central obrigatório.

Alpha 0.1.0-alpha.1: Windows x64 com PostgreSQL embarcado e Linux com Docker Compose. [Instalação e atualização](docs/installation.md), [plano](project-memory/plan.md), [decisões](project-memory/decisions.md) e [validação](project-memory/validation.md).

Implementado: setup protegido, login, catálogo básico, pedidos manuais, estoque transacional, snapshots de preços, deduplicação e fila local durável. Windows tem supervisor, tray, autostart opt-in, backup/restauração e diagnóstico.

Não representa o produto completo dos requisitos anexados. WhatsApp, Pix, iFood, IA, consentimentos, papéis granulares e catálogo avançado ainda não implementados. O painel apresenta integrações desabilitadas. Não há dados fictícios ou telemetria obrigatória.

Licença AGPL-3.0-only. O software não exige assinatura comercial; provedores externos opcionais têm seus próprios custos.
