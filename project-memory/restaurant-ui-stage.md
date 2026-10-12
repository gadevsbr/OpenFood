# DireÃ§Ã£o restaurante â€” 2026-10-11

Skills: UI Designer, UX Architect, UI Finish-Gate Reviewer.
UsuÃ¡rio: atendente encontra produtos por categoria, monta pedido e recebe;
gestor consulta resultado e mantÃ©m cadastros. Primeira leitura: cardÃ¡pio e pedido.

- Pedido explÃ­cito: aproximar o design da referÃªncia https://www.ifood.com.br/inicio.
  Site direto retornou 403/blank no Chromium; referÃªncia visual pÃºblica conferida:
  https://community.e-com.plus/t/transformar-banner-grid-em-slider-na-versao-mobile/4155.
- PadrÃµes adaptados: cabeÃ§alho branco; busca em superfÃ­cie cinza; categorias visuais;
  vermelho para seleÃ§Ã£o/aÃ§Ã£o; nomes, descriÃ§Ã£o e preÃ§os com hierarquia; navegaÃ§Ã£o leve.
  NÃ£o reproduzir banners promocionais de marketplace dentro do terminal de caixa.
- Verde floresta/lima e cartÃµes com iniciais do contrato anterior: **superseded**
  por neutros, vermelho e lista de cardÃ¡pio. Preservar fluxos e status semÃ¢nticos.
- Fotografias Unsplash apenas nas categorias reconhecidas; alternativas neutras para
  categorias personalizadas. Produtos exibem seus dados reais, sem fotos inventadas.
  Quatro JPEG locais embutidos; licenÃ§a em vendor/FOOD-IMAGES-LICENSE.txt.
- Mobile: lista de uma coluna, categorias horizontais, pedido acessÃ­vel no rodapÃ©.
  Claro/escuro/sistema, foco e movimento reduzido preservados.
- Proibidos: gradientes, glass, mÃ©tricas fictÃ­cias, fotos ilustrativas apresentadas
  como produtos reais, renomear categorias cadastradas, dependÃªncias CDN no uso.
- Gate: HOLD atÃ© capturas desktop/mobile/escuro e regressÃ£o PDV/mesas com PostgreSQL.
  Release experimental 0.9.0-alpha.1 planejada; nÃ£o publicada nesta etapa.
