# Audit de integrare — 2026-11-03

Produs: **wisemed-ai**. Audit efectuat înaintea implementării; schimbările preexistente din working tree au fost păstrate.

| Componentă verificată | Concluzie și reutilizare |
| --- | --- |
| `readersv3/core/module`, `core/runtime` | Contract Init/Start, registry de servicii, ServeMux și meniu. Modul nou `wisemed-ai`, înregistrat în `modules/builtin`. |
| `apps/runner` | Config bootstrap/merge, logger, lifecycle, servicii native și opțiuni standard. Utilitarul pornește cu SkipBootstrap și verificare updates în fundal. |
| `apps/html-print-formatter`, `apps/wisemed-siui-data` | Layout de utilitar, entrypoint, deployments/help și distribuție; nu s-au înlocuit implementările existente. |
| `modules/wisemedapi` | HTTPS, JWT aplicație; `PUT /administrative/login` cu răspuns `user_id/login_token`; `PUT /administrative/analyzer` pentru înregistrarea cerută de WSM. Login AI WiseMED opțional, server-to-server, mapping explicit. |
| `modules/ws` | Client WSS real, device key/token_file/token_endpoint, hello_ack, reconnect, ID WiseMED verificat. Nicio autentificare proprie paralelă inventată. |
| `shared/apibridge` | Principal server-attested în context, tenant/subiect, transport JSON `api.request`. Aliasurile AI păstrează RBAC și proprietarul sesiunii. |
| `docs/wss-control.md` | Scope-uri și configurare UI existente; nicio acordare automată de permisiuni. Evenimentele AI adaugă o metodă de publicare țintită în clientul existent. |
| `modules/localhttp` | Console/settings/logging comune. Cabinetul Vue separat la `/ai/`, cu auth proprie; formulare de infrastructură AI pentru administratorul instanței. |
| `shared/appupdates`, `tools/releasectl` | App ID `wisemed-ai`, stable, auto-download, cheie WiseMED comună. Build/installer/update pentru platformele suportate. Adăugat `sync-app` pentru regenerarea strict a aplicației vizate. |
| PostgreSQL SIUI Data local | Conexiune reutilizată exclusiv în config local ignorat de Git, permisiuni 0600. Nicio credențială în documentație, fixture sau installer. |
| Schema existentă WiseMED | Nu este modificată. Numai obiecte noi `wai_`; instalarea pgvector în baza partajată rămâne decizia operatorului. |
| HIS/Odoo/context pacient | Nu a fost găsit un contract verificat de autorizare pacient sau de consultație generică. Adaptor generic HTTPS allowlisted, livrare numai după aprobare. Nu sunt simulate endpointuri clinice reale. |

Standalone folosește bcrypt și tokenuri opace AI; nu reutilizează tokenurile WiseMED și nu acordă privilegii clinice din headere HTTP ori din `api:admin`. Referințele paciente standalone sunt declarații ale medicului, nu dovadă de drepturi în HIS. QR nu acordă acces în afara utilizatorului/tenantului autentificat.

Biblioteca de pornire este sintetică, fără pacienți sau rezultate normale implicite. Cele 52 de șabloane sunt versiuni independente, cu registru de seed per tenant; seed-ul repetat nu rescrie personalizările.
