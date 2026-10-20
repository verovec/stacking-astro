# AstroStack 2 — application macOS native de traitement astrophoto

*Conception produit et architecture, 2026-10-19. Remplace la conception « atelier de composition
web » du même jour (`~/.claude/plans/clever-chasing-llama.md`, epic E04), que le PO a écartée au
profit d'une application macOS complète.*

---

## 0. La demande du PO, telle quelle

> « Je veux une application macOS, je veux que nous nous approchions au maximum de ce que propose
> PixInsight. J'en ai marre de stacker avec un modèle, je veux intégrer à cette app des tutoriels
> me permettant de l'utiliser intelligemment sans être trop verbeux, juste ce qu'il faut. Je veux
> pouvoir intensifier certaines couches, je veux automatiser tout ce qui est répétitif entre chaque
> empilement, mais je veux avoir la main sur ce qui est ajustable en fonction de l'objet, des
> filtres, du type de caméra (MC ou MM). Je veux un vrai truc de pro qui me permette de rendre le
> meilleur de mes photos ; je veux aussi qu'on continue d'utiliser tous les softs qu'on utilise déjà
> (les algos de Siril, ou alors de l'open source). Je veux le meilleur soft d'empilement possible.
> Je suis ok pour nettoyer tout l'existant. Juste pas touche aux dossiers `input` et `output`. »

Exigences dérivées, numérotées pour que chaque epic s'y rattache :

| # | exigence | mesure de réussite |
|---|---|---|
| R1 | Application macOS native, sensation « pro » (PixInsight) | fenêtre document, aperçu GPU plein cadre à 60 i/s, panneaux de process, historique, raccourcis |
| R2 | Plus de « stack avec un modèle » : chaque réglage est visible, explicable, modifiable | aucun ajustement caché ; chaque valeur a une fiche « pourquoi » ; aucun agent IA dans la boucle par défaut |
| R3 | Tutoriels intégrés, concis | parcours « première pile » en 7 étapes ; fiche de 3 à 5 lignes par process ; jamais de modale non sollicitée |
| R4 | Intensifier des couches à la main | curseurs par couche (Hα, OIII, SII, L, R, G, B), aperçu instantané, valeur = effet |
| R5 | Automatiser le répétitif, garder la main sur l'ajustable | **profils** (objet × filtres × caméra) qui prédéfinissent un **pipeline** avec des **points d'arrêt** où l'humain reprend la main ; rejouer un pipeline sur une nouvelle nuit en un clic |
| R6 | Le meilleur empilement possible, sur Siril et l'open source | Siril 1.4.x pour calibration/registration/empilement/SPCC ; GraXpert ; StarNet (installé par l'utilisateur) ; aucune dépendance propriétaire |
| R7 | `input/` et `output/` intouchés | l'app lit `input/` et les anciens runs d'`output/` en lecture seule ; ses propres données vivent ailleurs |
| R8 | Multichrome (OSC) d'abord, mono ensuite | OSC BB+DB livré avant les profils mono LRGB+Ha |

---

## 1. État des lieux : ce qu'on garde, ce qu'on jette

### 1.1 Données (sacrées)

| dossier | taille | statut |
|---|---|---|
| `input/` | 73 Go | **lecture seule** : captures (M31-BB/DB, IC1848, NGC7000-BB/DB, M45, SH2-132 et leurs `-ngccal`) |
| `output/` | 132 Go | **lecture seule** : 32 runs avec `run.json`, modes `deepsky`/`nebula` uniquement ; l'app sait les ouvrir (masters, `rgb_base`, `syn_*`, finals) |
| `library/` | 26 Go | masters de calibration empilés (bias/darks/flats). **À conserver** : rebuildable depuis `input/` mais au prix d'heures de calcul ; non cité par le PO, à confirmer |
| `work/` | 0,8 Go | scratch, jetable |

### 1.2 Code (chiffres mesurés le 19/10)

| bloc | lignes hors tests | tests | verdict |
|---|---|---|---|
| `internal/pipeline` (deepsky/nebula, lanes, parité, multi-nuits, finish) | 22 400 | 10 600 | **garder le cœur** (≈ 60 %), jeter finish GIMP, superviseur IA, mosaic/comet/pano |
| `internal/{inspect,calib,grade,transient,siril,graxpert,starnet,fits,photom,noise,stackalg,stacknative,filters,imgops,channeldetect,dither,trail,setqa,rawmeta,rawconv,optics}` | ≈ 22 000 | ≈ 17 000 | **garder** : c'est la connaissance métier validée sur tes données |
| `internal/{solar,planetary,skypano,meteor,eclipsegeom,nightscape,comet,mosaic,mosaicplan,skyplan,skycat,astro,deepstars,starfield,panelgroup,pointing}` | ≈ 33 000 | ≈ 22 000 | **jeter** (aucun run de ces modes dans `output/`) ; reste dans l'historique git |
| `internal/{api,job,store,turns,llm,mcpserver,sysmon,report,localfs,thumb,preview,toolhealth,videoout,gimp,postprocess}` + `cmd/*` | ≈ 11 000 | ≈ 3 000 | **jeter** : HTTP/SSE, Postgres, agent IA, GIMP, MCP |
| `frontend/` (Vue 3) | 21 600 | — | **jeter** |
| Docker, Compose, migrations Postgres (47), `justfile` web/stack | — | — | **jeter** |

Décision structurante (§4) : le cœur Go garder-able (≈ 44 000 lignes + 28 000 de tests) devient un
**moteur batch embarqué** dans l'app ; tout le reste disparaît.

### 1.3 Outils hôte (vérifiés le 19/10)

| outil | état | rôle dans AstroStack 2 |
|---|---|---|
| Siril 1.4.4 (`Siril.app`, Python 3.12 + `python_module` embarqués) | présent | calibration, débayerisation, registration, empilement, drizzle, `pm`, SPCC/PCC, `subsky`, `autoghs`, déconvolution, ondelettes |
| GraXpert (`GraXpert.app`, CLI `-cmd background-extraction|denoising`) | présent | extraction de fond IA, débruitage IA |
| StarNet2 (`/usr/local/bin/starnet2`) | présent | séparation étoiles (licence non redistribuable : l'utilisateur l'installe) |
| GIMP 3.2.4 | présent | **plus utilisé** |
| Xcode | **absent** (Command Line Tools seuls, Swift 6.3) | nécessaire : compilateur Metal (`xcrun metal` introuvable), Instruments, signature |
| Machine | M5 Pro, 20 cœurs, 48 Go unifiés, Metal 4, macOS 26.5 | une image 22 Mpx × 5 plans float32 = 450 Mo : tient entièrement en mémoire GPU |

---

## 2. Modèle conceptuel (ce que PixInsight fait bien, traduit pour nous)

PixInsight repose sur une poignée de concepts qui expliquent sa puissance ; on les reprend sous
des noms français lisibles, en simplifiant ce qui ne sert pas un astrophotographe seul.

| PixInsight | AstroStack 2 | définition |
|---|---|---|
| Project | **Projet** | une cible (M 31). Contient ses nuits, ses images, son historique, ses recettes. Un dossier `.astrostack` versionné, hors `input/`/`output/` |
| WBPP | **Pré-traitement** | la partie automatique : classification → calibration → registration → notation → empilement → masters linéaires. Un **profil** la paramètre entièrement |
| ImageWindow + History | **Image** | plans float32 (1 ou 3+), métadonnées (filtre, caméra, grille), **historique non destructif** des process appliqués, retour arrière |
| Process | **Process** | un nœud paramétré : panneau de réglages, **aperçu temps réel**, bouton Appliquer, fiche d'aide. Catalogue en §3 |
| Process icon / instance | **Recette** | une instance de process avec ses valeurs, nommée, glissable sur une autre image, exportable |
| ProcessContainer / script | **Pipeline** | une chaîne ordonnée de recettes avec des **points d'arrêt** (« ici j'ajuste à la main ») ; rejouable sur une nouvelle nuit ou un autre projet |
| — (implicite dans les scripts) | **Profil** | objet × jeu de filtres × type de caméra (MC/MM) → pipeline + valeurs par défaut + fiches « pourquoi ». C'est la réponse à « automatiser le répétitif, garder la main sur l'ajustable » |
| Preview | **Région** | un rectangle d'aperçu 1:1 pour juger un réglage à la résolution native |
| STF | **Affichage** | autostretch d'affichage, indépendant des données (on regarde du linéaire sans l'étirer) |
| Mask | **Masque** | luminosité / étoiles / plage / dessiné ; restreint l'effet d'un process |
| PixelMath | **Calcul pixel** | expressions par canal (hors v1, prévu) |

Deux différences assumées avec PixInsight : (1) le pré-traitement n'est **pas** un script opaque :
chaque étape est un process du même catalogue, avec les mêmes fiches, exécuté par le moteur batch ;
(2) il n'y a **pas** de « magie » : aucun process ne mesure l'image pour changer ses propres
réglages en silence (règle R2). Les mesures sont affichées (bruit, SNR, écrêtage) et peuvent
remplir une **suggestion**, jamais une valeur.

---

## 3. Catalogue des process (v1 puis v2) et qui les exécute

Légende moteur : **S** = Siril 1.4 (via session persistante ou `siril-cli`), **G** = GraXpert,
**N** = StarNet, **M** = natif Metal (temps réel), **C** = cœur Go (batch).

### 3.1 Pré-traitement (automatique, paramétré par le profil)

| process | moteur | ce qu'on garde de l'existant |
|---|---|---|
| Inspection & classification des trames (lights/darks/flats/bias, filtre, gain, nuit, jeu BB/DB) | C | `internal/inspect`, `filterset.go`, `manifest.go`, `channeldetect` |
| Bibliothèque de calibration (masters par gain/offset/bin/exposition/température ; flats **par nuit**) | C | `internal/calib` (deep masters, `.sig`, defects) |
| Calibration des lights (bias/dark/flat, cosmétique, CFA-aware puis débayer) | S via C | scripts `internal/siril` |
| Registration (2 passes, homographie, **normalisation de parité** miroir) | S via C | `reuse_process.go` `parityCache`, `alignchannels.go` |
| Notation & rejet (FWHM, rondeur, fond, étoiles, trails Hough) | C | `internal/grade`, `internal/trail` |
| Masque de transitoires inter-trames (satellites, cosmiques) | C | `internal/transient` |
| Normalisation photométrique multi-nuits | C | `internal/photom` |
| Empilement (moyenne/médiane, rejets percentile/winsorisé/GESD, poids) | S (+ natif Go) | `internal/stackalg`, `stacknative` |
| Drizzle | S | nouveau (Siril 1.4 `-drizzle`) |
| Lanes broadband / dual-band, master DB recalé sur la grille BB, `syn_Ha`/`syn_OIII` | C | `filtersetlanes.go`, `duoband.go` |
| **Sortie = bundle linéaire** : couches float32 sur une grille, stats mesurées, provenance | C | nouveau (`bundle.json`, cf. conception E04) |

### 3.2 Process linéaires (interactifs ou automatiques selon le profil)

| process | moteur | notes |
|---|---|---|
| Extraction de fond | G (IA) / S (`subsky` RBF) / M (aperçu du modèle de fond) | choix explicite, pas de cascade silencieuse |
| Déconvolution | S (`rl`, `sb`, `wiener` 1.4) | PSF mesurée, aperçu sur région |
| Débruitage linéaire | G (denoise) / S (`denoise`) | force visible, avant/après |
| Calibration couleur | S (SPCC Gaia hors ligne, PCC) + M (neutralisation de fond) | **désactivée par profil sur dual-band** (physiquement invalide), avec la fiche qui dit pourquoi |
| Soustraction du continuum (Hα − k·R, OIII − k·G) | M | `k` mesuré affiché, modifiable |
| Combinaison des canaux : LRGB (poids, black par canal, L en luminance), HaRGB, HOO/SHO/HOS/Foraxx | M | port de `palette.go`, `hacontinuum.go` |
| Astrométrie (plate-solve) | S | pour SPCC et annotations |

### 3.3 Process non linéaires (le cœur du « rendre le meilleur »)

| process | moteur | notes |
|---|---|---|
| Étirement : MTF/autostretch, **GHS** (hyperbolique généralisé), arcsinh ; lié/délié ; headroom | M (+ S `autoghs` en référence) | temps réel plein cadre |
| HDR deux échelles (cœur de galaxie) | M | port de `hdrcomp` : seuils **absolus**, ouverture avant flou |
| Écrans d'émission Hα/OIII/SII : seuil (en σ du bruit mesuré), intensité, teinte, retrait des étoiles, boost | M | port d'`astrocomp` ; la réponse à R4 |
| Courbes & niveaux (luminance, RGB, saturation, par canal) | M | spline, points glissables |
| Saturation masquée, désaturation ciel (hue-sélective), lissage chroma | M | ports de `sky_desat*`, `chromasmooth` |
| Séparation d'étoiles | N | une fois par image ; couche étoiles persistée |
| Couche étoiles : réduction, désaturation (`k`), recomposition écran, tiers 25/50/75 | M | port de `stardesat`/`BlendStars` |
| Débruitage non linéaire | G | |
| Netteté / ondelettes / masque flou local | S (`wavelet`) / M (unsharp) | |
| Masques : luminosité, étoiles, plage, dessiné | M | |
| Recadrage, orientation, export (PNG 16, TIFF, FITS, JPEG) | M/C | noms de sortie libres ; profil « jeu de livrables » (final + starless + 25/50/75) |

### 3.4 Hors v1, prévu

Calcul pixel (expressions), mosaïque, comète, planétaire, panoramas, vidéo. Le modèle (Projet /
Process / Pipeline) les accueille sans refonte ; le code historique reste dans git.

---

## 4. Architecture logicielle

### 4.1 Vue d'ensemble

```
┌──────────────────────────── AstroStack.app (Swift 6 · SwiftUI · Metal 4) ────────────────────────────┐
│  UI            Projet · Pré-traitement · Studio (process + aperçu GPU) · Pipelines/Profils · Aide  │
│  ProcessKit    registre des process, schémas de paramètres, recettes, pipelines, points d'arrêt     │
│  RenderKit     shaders Metal : STF, étirements, écrans, HDR, courbes, saturation, masques, étoiles   │
│  ImageKit      FITS/TIFF I/O, plans float32, pyramide mip, tuiles, historique non destructif        │
│  ProjectKit    modèle de projet (SQLite), recettes, profils, tutoriels (contenu versionné)          │
│  CoreClient    client du moteur batch : JSON-RPC sur socket Unix, événements de progression         │
└───────────────────────────────┬──────────────────────────────────────────────────────────────────────┘
                                │ Contents/Helpers/astrocore  (processus enfant, un par app)
┌───────────────────────────────▼──────────────────────────────────────────────────────────────────────┐
│  astrocore (Go, extrait de l'existant)                                                               │
│  inspect · calib · grade · transient · photom · stackalg/stacknative · lanes/parité · bundle          │
│  pilotes : Siril (session persistante Python ou siril-cli) · GraXpert · StarNet                      │
└──────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Pourquoi Swift + Metal pour l'app (R1, R4)

Le GPU unifié d'un M5 Pro rend une image 22 Mpx float32 à chaque mouvement de curseur en quelques
millisecondes ; c'est ce qui donne la sensation PixInsight (aperçu plein cadre, pas un proxy). SwiftUI
+ AppKit fournissent la fenêtre document, les panneaux, l'inspecteur, les raccourcis, Quick Look,
glisser-déposer depuis le Finder. Swift 6 est déjà sur la machine ; **Xcode doit être installé**
(compilateur Metal hors ligne, Instruments pour profiler le GPU, signature). Alternative écartée :
Tauri/Electron (web embarqué : on retomberait sur WebGPU et un rendu moins intégré) ; Fyne/Gio en Go
(pas au niveau pour une UI image intensive).

### 4.3 Pourquoi garder un moteur Go embarqué (R6, et le risque)

Le pré-traitement n'a pas besoin d'interactivité ; il a besoin de **justesse**. Les 44 000 lignes
retenues encodent deux ans de cas réels sur tes données : flats par nuit, calibration dimension-
gardée, parité miroir d'un train optique, lanes BB/DB, masque de transitoires sur couloir
réutilisé, normalisation photométrique sans écrêtage, classification BB/DB en électrons. Les réécrire
en Swift coûterait des mois et réintroduirait des régressions déjà payées. Le pattern « helper
embarqué » (binaire dans `Contents/Helpers`, lancé par l'app, protocole local) est standard sur
macOS. Ce qui change pour ce code : plus de HTTP, plus de Postgres (SQLite), plus de jobs web ; un
**protocole typé** (JSON-RPC 2.0 sur socket Unix, événements `progress`/`log`/`preview`), un
processus par app, annulation par requête.

Variante « tout Swift » (si le PO la préfère) : même modèle, le moteur devient un module Swift ;
coût estimé 3 à 4 fois celui de l'extraction, bénéfice : un seul langage. Recommandation : **hybride
d'abord**, migration opportuniste process par process vers Metal/Swift quand l'interactivité le
justifie (c'est exactement ce que fait la §3 : tout le non-linéaire est déjà natif).

### 4.4 Pilotage de Siril (vérifié dans les sources et la doc Siril, 19/10)

- **Session persistante = mode tube nommé** : `siril-cli -p` lit les commandes sur
  `siril_command.in` et écrit `ready` / `log:` / `status: starting|success|error|exit` /
  `progress: N%` sur `siril_command.out` (chemins personnalisables `-r/-w`, FIFO créés par nous).
  `astrocore` garde **une** instance Siril vivante par session de pré-traitement, envoie les
  commandes une à une et lit la progression en flux — le parseur `progress:`/`log:`/`status:`
  existe déjà (`internal/siril/runner.go`). Repli byte-identique : `siril-cli -s script.ssf` par
  étape (`internal/siril/scripts.go`).
- Le module Python `sirilpy` **n'est pas** un canal applicatif : son socket est créé par Siril à
  chaque `pyscript`, nommé par UUID et transmis par variable d'environnement au script lancé. On
  l'utilise seulement via `pyscript` pour les enrobages officiels Siril de GraXpert
  (`GraXpert-AI.py -bge|-denoise|-deconv_obj|-deconv_stellar`) et StarNet ≥ 2.5 (`StarNet.py`) si
  l'appel direct des binaires pose problème.
- Commandes 1.4 confirmées disponibles en ligne : `autostretch`, `autoghs`/`ght`/`modasinh`
  (GHS), `pm` (calcul pixel), `spcc`/`pcc`/`platesolve` (Gaia hors ligne par chunks Zenodo, déjà
  dans `library/catalogues`), `subsky -rbf`, `rl`/`sb`/`wiener` + `makepsf` (déconvolution),
  `denoise` (NL-Bayes), `rgbcomp`, `seqstarnet`, drizzle au `register`/`stack`.
- **Absent en ligne de commande** : la recomposition d'étoiles (GUI seulement) — donc native Metal
  (§3.3), comme prévu. Le `starnet` intégré C est déprécié en 1.4 : on appelle `starnet2`
  directement (existant) ou via `pyscript`.

### 4.4b Outils IA open source : ce qu'on prend, ce qu'on refuse

| outil | licence | verdict |
|---|---|---|
| GraXpert 3.0.x (installé) : fond, débruitage | GPL-3 | **oui**, CLI directe (`-cli -cmd …`, GPU CoreML) |
| GraXpert 3.1 RC : déconvolution objet/stellaire (`-cmd deconv-obj|deconv-stellar`) | GPL-3 | **oui dès stable** : c'est la seule déconvolution IA libre, Apple Silicon, en CLI ; mise à jour de l'install utilisateur |
| Siril `rl`/`sb`/`wiener` + `makepsf`, `denoise` | GPL-3 | **oui**, déterministes, scriptables, référence quand l'IA n'est pas voulue |
| StarNet2 2.6 | propriétaire gratuit, pas de licence publiée | **invoqué, jamais embarqué** (comme aujourd'hui) ; process grisé + fiche d'installation s'il manque |
| Seti Astro Cosmic Clarity | MIT mais **archivé** (09/2026), successeur à modèles propriétaires | **non** : ne pas bâtir dessus |
| ASTAP / `arcsec` (résolution astrométrique hors ligne) | MPL-2 / MIT | **optionnel, embarquable** : repli si le solveur Siril échoue sur un champ (cas connu du 20 mm) |
| XISF (`libXISF` C++ GPL) | GPL | **non** : FITS float32 reste le format pivot ; XISF en export seulement si un jour utile |

### 4.5 Données et fichiers

- **Projets** : `~/AstroStack/Projects/<Objet>.astrostack/` (SQLite `project.db` + `images/` en FITS
  float32 + `recipes/` + `history/`). Jamais dans `input/` ni `output/`.
- **Import** : `input/<...>` référencé par chemin, jamais copié ni modifié ; les anciens runs
  `output/<Objet>/<run>/` s'ouvrent en lecture seule comme **source de bundle** (masters,
  `rgb_base.fits`, `syn_*.fits`) pour rejouer un rendu sans re-stacker.
- **Bibliothèque de calibration** : `library/` réutilisée telle quelle (mêmes clés `g{gain}o{offset}_b{bin}`).
- **Scratch** : `~/Library/Caches/AstroStack/` (budget et nettoyage visibles dans l'app — le PO a déjà
  eu ~0,65 Go/pose de scratch non nettoyé).

### 4.5b Lecture/écriture d'images côté Swift

Aucune bibliothèque Swift FITS mature (FITSCore v0.1, FITSKit iOS seulement) : **ImageKit écrit son
propre lecteur/écrivain FITS** (en-têtes de 80 caractères, BITPIX −32/16/8, NAXIS3, ROWORDER,
cartes WCS conservées — ~400 lignes, testées contre les fichiers Siril de `output/`). TIFF/PNG 16 bits
via ImageIO. Le cœur Go garde son `internal/fits` ; un golden croisé prouve que les deux lisent le
même fichier à l'identique.

### 4.6 Pixel et aperçu

Plans float32, pyramide mip calculée à l'import (moyennes par blocs), textures Metal `r32Float`.
Chaîne d'affichage : image → process en cours (shader) → STF/affichage → écran, à chaque frame. Les
process natifs (§3.2–3.3 colonne M) tournent en plein cadre ; les process batch (S/G/N) s'exécutent
sur l'image et reviennent dans l'historique. **Règle d'exactitude** : ce que l'aperçu montre est ce
qu'« Appliquer » écrit (même shader, même précision), sauf pour S/G/N dont l'aperçu est l'image
résultat elle-même (pas d'approximation). **Limite honnête** (revue 2026-10-20) : un process dont un
paramètre est en pixels (ouverture, flou, rayon d'étoile) n'est exact qu'à 1:1 et dans la région ;
sur un niveau de pyramide, l'opérateur morphologique ne commute pas avec le sous-échantillonnage.
Ces process portent le drapeau `previewExactOnlyAt1to1` et le Studio affiche « aperçu approximatif
hors 1:1 » tant qu'on n'est pas à l'échelle native.

---

### 4.7 Construction, signature, distribution

- **Xcode 26 + composant Metal Toolchain** (`xcodebuild -downloadComponent metalToolchain`) pour
  compiler les `.metal` en `.metallib` ; en développement sans Xcode, repli
  `MTLDevice.makeLibrary(source:)` (compilation à l'exécution, démarrage plus lent, pas de
  profilage).
- Projet Xcode généré depuis SwiftPM (`Package.swift` = source de vérité des modules) ; `just`
  recettes : `just app-build`, `just app-run`, `just app-test`, `just core-build`, `just check`.
- Le helper Go est compilé (`CGO_ENABLED=0`, arm64) dans `Contents/Helpers/astrocore`, **signé
  individuellement** (Developer ID, hardened runtime, horodatage) puis l'app ; pas de sandbox App
  Store (accès libre à `input/`, `output/`, `library/`, Siril, GraXpert, StarNet). Distribution :
  DMG notarisé, ou build local non signé pour le PO seul (première livraison).

## 5. Parcours utilisateur (ce que tu fais, dans l'ordre)

**A · Nouvelle nuit sur M 31.** Fichier → Importer → tu choisis `input/M31-DB/2026-10-18` ; l'app
classe (lights dual-band, 34 poses, gain 100, −20 °C), propose le **profil** « Galaxie · OSC ·
broadband + dual-band » déjà attaché au projet, montre les masters de calibration qu'elle va
utiliser (flat de la nuit, dark 300 s −20 °C) et ce qui manque. Un bouton : **Pré-traiter**.
Progression par étape, journal, aperçus. Fin : le bundle linéaire est mis à jour (76 + 34 poses) et
le pipeline s'arrête au **premier point d'arrêt** du profil.

**B · Studio.** À gauche la pile des process du pipeline (faits / en cours / à venir), au centre
l'image plein cadre GPU, à droite le panneau du process courant : « Écrans d'émission » — couche Hα :
intensité 0,95, seuil 0,115 (= 3,1 σ), couche OIII… Tu bouges, l'image suit. Fiche d'aide : 4 lignes.
Appliquer → historique. Suivant : HDR cœur, saturation, séparation d'étoiles (StarNet, 90 s, barre),
couche étoiles, export du jeu de livrables.

**C · Rejouer.** La semaine suivante, 40 poses de plus : Importer → Pré-traiter → le pipeline rejoue
**toutes** tes recettes jusqu'au point d'arrêt que tu as choisi (« tout automatique » ou « stop avant
les écrans »), avec tes valeurs. Diff visible entre l'ancien rendu et le nouveau.

**D · Autre cible, même matériel.** Nouveau projet IC 1848 → profil « Nébuleuse émission · OSC ·
dual-band seul » : calibration couleur désactivée (fiche : pourquoi), lissages chroma à 0, desat
douce 0,62 proposée. Tu ajustes, tu enregistres **ta** recette : elle devient le défaut de ce profil
pour toi.

---

## 6. Tutoriels et aide (R3)

Trois niveaux, tous **à la demande**, en français et en anglais, contenu versionné dans l'app :

1. **Premiers pas** (7 étapes, une fois) : importer, lire la classification, pré-traiter, lire le
   bundle, premier étirement, premier écran, exporter. Déclenché par le bouton « Guide », jamais seul.
2. **Fiche de process** (3 à 5 lignes, visible dans le panneau) : *quoi* (une phrase), *quand*
   (objet/filtre), *quel curseur en premier*, *piège connu*. Exemple, Calibration couleur : « SPCC
   force les étoiles vers leurs couleurs catalogue en large bande. En dual-band, les étoiles ne
   brillent que dans deux fenêtres étroites : SPCC "corrige" un excès de rouge qui n'existe pas et
   refroidit le Hα. Désactivée dans ce profil. »
3. **Pourquoi cette valeur** (une ligne, au survol de chaque défaut d'un profil) : « 0,115 : seuil
   validé le 19/10 sur M 31 ; plus bas = voile rouge diffus, plus haut = nœuds HII seuls. »

Pas de vidéo, pas de modale bloquante, pas de chat.

---

## 7. Profils et automatisation (R5)

Un profil = `{objet, filtres, caméra} → pipeline + valeurs + fiches`. Livrés en v1 (dérivés de tes
recettes validées) :

| profil | pipeline notable |
|---|---|
| Galaxie · OSC · BB + DB (M 31) | lanes BB/DB, SPCC sur BB, continuum-sub, HDR cœur on, écrans HaPlus (0,115 / 0,95), OIII 0,80 boost 1,25, sat 0,32 masquée, étoiles k 0,25 |
| Nébuleuse émission · OSC · DB seul (IC 1848) | pas de SPCC, lissages chroma 0, étirement délié, desat 0,62, HDR off |
| Amas / réflexion · OSC · BB (M 45) | base seule, sat basse, désaturation d'étoiles, pas d'écran |
| Galaxie · mono · LRGB + Hα (FC-100 + ASI 1600MM) | poids RGB, L en luminance, Hα continuum-sub en écran |
| Nébuleuse · mono · SHO / HOO | palettes, étirement délié par canal |

Mécanique : **points d'arrêt** par étape (`auto` / `arrêt`), **verrouillage** d'une étape (ne pas
rejouer), **rejeu** sur nouvelle nuit ou nouveau projet, **diff** de deux pipelines, **export/import**
de recettes (JSON) pour partager entre projets. Les profils livrés sont des points de départ ; tes
sauvegardes les remplacent pour toi sans les écraser.

---

## 8. Qualité : comment on sait que c'est « le meilleur »

- **Parité** : chaque process natif porté depuis `astrocomp`/`hdrcomp`/`stardesat` est validé par un
  golden contre le livrable `M31_v9style_hdr_HaPlus` du run #78 (|Δ| moyen ≤ 1/255).
- **Byte-identité du pré-traitement** : les masters produits par `astrocore` sont identiques à ceux
  du moteur actuel sur les goldens existants (`TestProcess_SingleNightGolden`, deux nuits ancrées).
- **Mesures affichées, pas jugées** : bruit (MADN), SNR par couche, écrêtage, FWHM, excentricité,
  avant/après par process — l'app te donne les chiffres, elle ne décide pas.
- **Perf** : aperçu plein cadre 22 Mpx ≤ 16 ms GPU pour tout process natif ; import + pyramide d'une
  image < 2 s ; pré-traitement ≥ aussi rapide qu'aujourd'hui.
- **Acceptation PO** par cible de référence (M 31, IC 1848, M 45) avant chaque bascule de défaut.

---

## 9. Risques et comment on les tient

| risque | mitigation |
|---|---|
| Xcode absent : pas de compilateur Metal, pas d'Instruments | prérequis humain : installer Xcode (+ composant Metal Toolchain) ; repli technique : compilation des shaders **à l'exécution** (`makeLibrary(source:)`) pour développer sans Xcode, à confirmer |
| Mode tube nommé `siril-cli -p` : seules les commandes « scriptables » sont acceptées, un plantage Siril tue la session | liste des commandes pinnée par un test live de grammaire (même discipline que `TestSirilLive_StackClauseGrammar`) ; relance automatique de l'instance et reprise à l'étape ; repli byte-identique sur `siril-cli -s` |
| StarNet non redistribuable | jamais embarqué : détecté, sinon le process est grisé avec la fiche d'installation |
| GraXpert local sans déconvolution | déconvolution par Siril ; GraXpert pour fond + débruitage |
| 132 Go d'anciens runs à ne pas corrompre | ouverture en lecture seule, aucune écriture dans `output/` par construction (pas de droit d'écriture demandé) |
| Volume de travail (app + extraction + process) | découpage en epics indépendamment livrables ; première valeur visible dès E11 + E13 (ouvrir un ancien run et régler les écrans en temps réel) |
| Un seul développeur-agent, exigence « pro » | TDD par carte, goldens de parité, revue de code, acceptation PO par cible |

---

## 10. Découpage en epics

| epic | titre | livre | dépend |
|---|---|---|---|
| **E10** | Fondations app macOS | projet SwiftPM/Xcode, fenêtre document, ImageKit (FITS float32, pyramide), RenderKit (STF, zoom/pan, région 1:1, histogramme), ouverture d'un ancien run d'`output/` en lecture seule | Xcode installé |
| **E11** | Moteur `astrocore` | extraction du cœur Go, JSON-RPC sur socket, suppression HTTP/Postgres/web/Docker/GIMP/IA, bundle linéaire, SQLite, byte-identité des masters | — |
| **E12** | Studio : process natifs | étirements (MTF/GHS/arcsinh), écrans d'émission, HDR, courbes, saturation/désat, combinaison des canaux, continuum-sub, masques, historique non destructif, Appliquer/Annuler ; parité v9 | E10 |
| **E13** | Étoiles & export | StarNet, couche étoiles, recomposition, tiers, recadrage/orientation, jeu de livrables | E12 |
| **E14** | Pré-traitement dans l'app | import depuis `input/`, classification, bibliothèque de calibration + inspection des masters, revue des trames (métriques, blink, garder/rejeter, trame de référence), réglages d'empilement simple/avancé, exécution via `astrocore`, progression, bundle | E10, E11 |
| **E15** | Pipelines, recettes, profils | recettes, pipelines à points d'arrêt, profils livrés, rejeu, verrouillage, diff, import/export | E12, E14 |
| **E16** | Tutoriels & fiches | premiers pas, fiches de process, « pourquoi cette valeur », fr/en, contenu versionné | E12 |
| **E17** | Process batch avancés | extraction de fond (GraXpert/subsky, aperçu du modèle — livrée en v1), déconvolution Siril, débruitage GraXpert/Siril, SPCC/PCC, plate-solve, ondelettes, drizzle | E14 |
| **E18** | Mono | rôles R/G/B/L, profils mono, palettes NB | E12, E14 |
| **E19** | Nettoyage & livraison | suppression du legacy (web, Docker, Postgres, modes hors deep-sky), nouveau `CLAUDE.md`/README, signature/notarisation, DMG | E14, E15 |

Ordre retenu après la revue du 2026-10-20 : **pré-traitement d'abord**. La file commence par
E11 (carve, RPC, SQLite) + l'import E14, puis le viewer E10 et l'exécution E14 : l'app stacke une
nuit depuis `input/` à la 12e carte, avec revue des trames, réglages d'empilement et inspection des
masters (R2 vaut aussi dans le pré-traitement : aucun verdict que l'utilisateur ne peut voir et
contredire). Le Studio E12 vient ensuite sur un bundle produit par l'app elle-même ; les oracles de
parité (astrocomp/hdrcomp/stardesat) sont versionnés dans le dépôt avant le premier port.
L'ancienne première valeur (ouvrir le run #78 et régler les écrans) reste la première valeur du
Studio, elle n'est plus la première valeur de l'app.

---

## 11. Décisions prises par le PO (19/10)

1. **Hybride** : app Swift/SwiftUI/Metal + moteur Go embarqué (`astrocore`) — §4.3.
2. **Périmètre v1 = deep-sky, OSC puis mono** ; planétaire, solaire, panoramas, comète, mosaïque,
   vidéo sortent de l'app (code conservé dans l'historique git).
3. **`library/` (26 Go) conservée**, au même titre qu'`input/` et `output/`.
4. **Xcode sera installé** par le PO (prérequis d'E10 ; composant Metal Toolchain inclus).
5. Hypothèse retenue sans question : **monorepo** — l'app vit dans ce dépôt (`apps/macos/`,
   `core/` pour le Go), l'historique est conservé, le legacy est supprimé par E19.
