# CLAUDE.md — K9 Guard (dbwatch)

Guía para modificar la app con Claude Code. El manual de uso está en `README.md`.

## Qué es

Un solo binario Go (`dbwatch`) que es a la vez:

- **App de macOS** (Fyne): ventana con lista de consultas, gráfica y visor de CSV, más ícono en la barra de menú. Empaquetada como `~/Applications/K9 Guard.app`.
- **CLI**: `connections`, `add`, `list`, `show`, `run`, `enable/disable`, `seen`, `remove`, `ui`, `tray`, `serve`, `install-agent`, `notify-test`.

Ejecuta consultas PostgreSQL **de solo lectura** sobre las conexiones de DataGrip, con un horario. Guarda cada resultado (CSV e historial), evalúa una regla sobre la cantidad de filas y notifica.

## Compilar, probar y ver cambios

```bash
go vet ./... && go test ./...            # siempre antes de dar algo por terminado
gofmt -l .                                # debe salir vacío; si no: gofmt -w *.go
./build-app.sh                            # compila y reemplaza ~/Applications/K9 Guard.app
pkill -f "K9 Guard.app/Contents/MacOS/dbwatch"; sleep 1; open "$HOME/Applications/K9 Guard.app"
tail -f ~/.dbwatch/app.log                # log de la app abierta desde Finder
```

- El linker muestra `ld: warning: ignoring duplicate libraries: '-lobjc'`; es inofensivo.
- Para probar la CLI sin tocar los datos reales: `DBWATCH_HOME=$(mktemp -d) ./dbwatch ...`.
- **Verificación visual sin pantalla:** Claude Code no puede capturar la pantalla (no tiene permiso de grabación). Para revisar un widget, se escribe un test temporal que lo dibuja con `fyne.io/fyne/v2/test` (`test.NewApp()`, `a.Settings().SetTheme(theme.DefaultTheme())`, `test.NewWindow(obj)`, `w.Canvas().Capture()`) y guarda un PNG en el scratchpad. Se revisa la imagen y se borra el test. Así se validaron la gráfica, los íconos y la bienvenida.
- Probar contra la BD local con `Localhost` (`postgres`, `booking`, schema `ixtul`). `SELECT g FROM generate_series(1, @n) AS g` sirve para forzar cantidades.

## Convenciones del código

- **Sin comentarios en el código** (regla del usuario): ni encabezados, ni docstrings, ni de línea. La documentación vive aquí y en el README.
- Textos de la interfaz y mensajes de error **en español**; nombres de identificadores en inglés.
- Fechas visibles: `timestampLayout = "2006-01-02 15:04:05"`.
- Antes de editar, leer el archivo vecino e imitar su estilo. Archivos chicos por responsabilidad (ver tabla).
- Toda actualización de widgets desde una goroutine va dentro de `fyne.Do(...)`.
- Probar cada lógica nueva sin UI con un `*_test.go` (ver los existentes); la UI se valida con el render en PNG.

## Archivos

| Archivo | Responsabilidad |
|---|---|
| `main.go` | CLI: parseo de comandos y flags, `install-agent` (LaunchAgent `com.dbwatch.serve`, `KeepAlive.SuccessfulExit=false`). Sin argumentos dentro de un `.app`, abre la UI. |
| `store.go` | Modelo `Query`, `QueryState` y `Param`; `queries.json` y `state.json`; `updateState` (mutex), `markSeen`, `setEnabled`, `clearQueryData` (botón Limpiar: estado + historial + CSV); `duplicateQuery`/`copyName` (menú Duplicar); reglas `Op()`, `Matches()`, `RuleText()`, `RowLimit()`, `SchemaRowLimit()` (tope por schema si `PerSchemaMaxRows`); `numberRows` (gui.go) antepone la columna `#` solo en pantalla; `truncateDecimals` corta a 4 decimales solo en pantalla (también lo que copia el menú); `contains`. |
| `sources.go` | Fuentes de conexiones: descubre proyectos de DataGrip, extras y globales; `settings.json`; `activeSources`. |
| `datagrip.go` | Lee `dataSources.xml`, `dataSources.local.xml` y `sshConfigs.xml`; arma `Connection`; desambigua nombres repetidos `Nombre (Proyecto)`; contraseña del llavero (`security find-generic-password -s "IntelliJ Platform DB — <uuid>" -w`). |
| `tunnel.go` | `ssh -L` en un puerto libre por ejecución (`BatchMode`, espera hasta 20 s) y cierre. |
| `multischema.go` | Modo multiesquema: `targetSchemas` (lista o todos), revisión de `USAGE`, un savepoint por schema con `SET LOCAL search_path`, `schemaQuery` (fuerza `@schema`), avisos agrupados en `Result.Warnings` / `QueryState.Warnings`; error solo si fallan todos. |
| `db.go` | Conexión pgx, ejecución **read-only**, tope de filas con conteo total (`Result.TotalRows`, `Truncated`, `Count()`), `effectiveParams` (agrega `@schema`), listado de bases y schemas. |
| `params.go` | Parámetros estilo T-SQL `@Nombre`: escáner que salta cadenas, identificadores entre comillas, comentarios y `$$`; inferencia de tipo; fechas relativas. |
| `schedule.go` | Horario: `parseSchedule` (horas o `HH:MM[,HH:MM]`), `due`, ponerse al día, `observed` (una consulta nueva no corre por horas ya pasadas). |
| `scheduler.go` | `execute` (corre, guarda CSV e historial, evalúa la regla, notifica, actualiza estado), `evaluate` (regla pura y "solo si cambia"), `runDue`, `serve`, `executeExclusive` (mutex de ejecución). |
| `notify.go` | `notify(title, body, query)`: nativa si hay permiso; si no, osascript. CSV por ejecución (`YYYYMMDD-HHMMSS.000.csv` + `.meta.json`), poda a 50, `readCSV`, `csvLabel`. |
| `notify_darwin.go` / `notify_darwin.m` | Puente cgo a `UNUserNotificationCenter`: permiso, envío con `userInfo.query`, delegado de clic (activa la app y llama a Go) y presentación con la app en primer plano. |
| `history.go` | `history/<consulta>.jsonl` (cantidad por ejecución, 5000 puntos); relleno desde los CSV la primera vez; reemplaza el punto si es la misma ejecución (< 1 s). |
| `gui.go` | Ventana principal: lista (filas arrastrables con ▶ y toggle), bienvenida, pestañas Gráfica/Detalle, buscador, zona Eliminar, menú de la barra, diálogo de fuentes, clic en notificaciones. |
| `editor.go` | Formulario en `widget.NewModalPopUp`: encabezado con 🐞/💾/✕, listas en cascada de conexión→base→schema con caché por sesión, Esc para cerrar, tabla de prueba visible solo tras Probar. |
| `chart.go` | Widget de gráfica de línea propio (Fyne no trae gráficas): ejes, umbral, puntos coloreados y hover. |
| `widgets.go` | Widgets propios: `toggle`, `dragRow`, `tipButton` y `tooltipLayer`, `dragRow` también atiende el clic (`onTap` → `selectInList`) y el clic derecho (`onMenu` → Duplicar), porque al ser `SecondaryTappable` el clic ya no llega al item de la lista; `copyCell` (clic en cualquier celda marca su fila vía `onTap`; clic derecho abre el menú Copiar) y `toastLayer` (aviso temporal dentro de la app), `escEntry`, `foregroundTheme`/`tinted`, `scaledTheme`, espaciadores, colores (`debugColor`, `saveColor`, `cancelColor`, `seenColor`). |
| `icon.go` | Íconos incrustados (`assets/icon.png`, `assets/bug.svg`); ícono de la barra generado (escudo coloreado, líneas transparentes, recortado). |
| `build-app.sh` | Compila y arma el `.app` (Info.plist con `LSUIElement`, `.icns` desde el PNG, firma ad hoc). |

## Flujo de una ejecución

`runDue` (cada 30 s, salvo pausa) → `due(q, state, now)` → `executeExclusive` → `execute`:

1. `runQuery`: `ensureReadOnlySQL` → `renderSQL(effectiveParams(q))` → `findConnection` → túnel si hace falta → `pgx.ConnectConfig` (`default_transaction_read_only=on`, `statement_timeout=300000`, `search_path` = schema) → `BeginTx(ReadOnly)` → lee hasta `RowLimit()` y sigue contando.
2. Si no hubo error: `saveResultCSV` (+ meta) y poda.
3. `evaluate`: `Matches(count)` y, con `OnChange`, si la cantidad es igual a la anterior. Si no cambió, no notifica y conserva `Seen`.
4. `notify(...)` → nativa (`userInfo.query`) u osascript.
5. `updateState` (reemplaza el estado; `Seen=false` salvo el caso "sin cambios") y `appendHistory`.
6. La UI se refresca con `fyne.Do(g.reload)`.

## Modelo de datos

- `Query`: `name`, `connection` (nombre visible de la conexión), `database`, `schema`, `sql`, `params[] {name,type,value}`, `everyHours` o `at[]` (`"HH:MM"`), `threshold`, `operator` (vacío = `>`), `notifyOnChange`, `maxRows` (0 = 50 000), `enabled`.
- `Query` multiesquema: `schemas[]` o `allSchemas` (`MultiSchema()`, `SchemaText()`). `QueryState`: `warnings[]` (avisos de schemas omitidos), `lastRun`, `lastRows` (total real), `lastError`, `triggered`, `notified`, `seen`; `Alert()` = (`triggered` o error) y no `seen`.
- Agregar un campo: struct en `store.go` con tag JSON (`omitempty` si es opcional) → valor por defecto que no rompa los `queries.json` existentes → `cmdAdd`/`cmdShow` en `main.go` → `editor.build/load/query` → test.

## Trampas aprendidas (Fyne 2.8 y macOS)

- **No hay tooltips en Fyne.** `widget.PopUp` cubre la ventana y le roba el hover al botón, lo que causa parpadeo. Se usa `tooltipLayer`: un rectángulo y un texto no interactivos encima del contenido, en un `container.NewStack(contenido, layer)`. Dentro de un popup modal hace falta una capa propia (la de la ventana queda debajo); `Show` convierte la posición absoluta a coordenadas de la capa.
- **`container.NewHBox` estira a lo alto** a sus hijos: un botón dentro de una fila crece al alto de la fila. Envolver en `container.NewCenter`.
- **`widget.Select.SetSelected` dispara `OnChanged`.** Para cambiar opciones o selección sin efectos usar `selectQuietly` (quita `OnChanged`, cambia y lo restaura).
- **Una `widget.Label` vacía ocupa una fila completa.** Ocultarla (`Hide`) cuando no tiene texto, como en `editor.setStatus`. Lo mismo pasa con una fila vacía de `widget.Form`: para espaciar se pone un `verticalSpacer` dentro del contenido de la fila (`Border`), descontando `theme.Padding()`.
- **Colores de texto o ícono por widget:** `container.NewThemeOverride` con `foregroundTheme` (helper `tinted`). Tamaños: `scaledTheme`. `ThemeOverride` expone su hijo en `.Content`.
- **Los íconos del tema son SVG temáticos.** Un ícono propio que tome el color debe ser SVG y `theme.NewThemedResource` (ver `bugIcon`). Los PNG no se recolorean.
- **Ícono de la barra:** Fyne solo usa ícono plantilla con `ThemedResource`; con PNG se usa uno ya coloreado (`trayIcon`, blanco para la barra oscura y rojo con alertas).
- **Esc:** si un `Entry` tiene el foco, la tecla no llega a `Canvas.OnTypedKey`. Por eso los campos del editor son `escEntry`, y al abrir el popup se instala también `SetOnTypedKey` en el canvas (y se restaura al cerrar).
- **Columnas redimensionables:** Fyne solo permite arrastrar el ancho desde el encabezado nativo (`widget.NewTableWithHeaders` + `CreateHeader`/`UpdateHeader`, celdas de encabezado con `Row == -1`; `ShowHeaderColumn = false`). El ancho inicial lo calcula `columnWidth` (caracteres de las primeras 200 filas, entre 80 y 420). `showResult` recalcula los anchos al cargar otro resultado.
- **Copiar celdas:** cada celda es un `copyCell` con `TappedSecondary` (menú Copiar). No implementar `DoubleTapped`: el driver entrega los clics a la celda y no a la tabla, y retrasa el clic sencillo; por eso el clic también lo atiende la celda (`onTap`) y `Table.OnSelected` no se usa. Portapapeles con `fyne.CurrentApp().Clipboard()` (el de `Window` está deprecado). El aviso es un `toastLayer` (no interactivo, se oculta con `time.AfterFunc` + `fyne.Do`) apilado en la ventana y en el popup del editor.
- **Fyne no tiene drag & drop entre widgets.** `dragRow` implementa `fyne.Draggable` y compara `DragEvent.AbsolutePosition` con la zona Eliminar (`Driver().AbsolutePositionForObject`).
- **Plantilla de fila de `widget.List`:** la función de actualización recupera los widgets por posición (`Border` guarda primero el centro y luego los bordes). Si se cambia la estructura de la fila, actualizar los índices.
- **Notificaciones:** `osascript` las atribuye a Editor de Scripts y al hacer clic abre un selector de archivos. Las nativas (`UNUserNotificationCenter`) requieren correr como `.app` con bundle id; el permiso es asíncrono y antes de concederse se usa osascript. Sin delegado propio no hay clic ni banner con la app activa.
- **cgo:** un archivo con `//export` no puede tener definiciones en su preámbulo, por eso el Objective-C vive en `notify_darwin.m` (incluye `_cgo_export.h`).
- **Permisos por esquema:** sin `USAGE` sobre un esquema, PostgreSQL lo omite del `search_path` sin error y las tablas sin prefijo caen en otro esquema (antes en `public`), devolviendo datos ajenos o vacíos; `information_schema` tampoco lista tablas sin privilegio. Por eso `search_path` es solo el esquema elegido y `runQuery` valida que exista y que haya `USAGE` antes de ejecutar. `classifyError` convierte esa falla y cualquier `pgconn.PgError` con código `42501` en `Permisos insuficientes: <mensaje>` (`permissionPrefix`); la UI lo detecta con `isPermissionError` y muestra la franja mostaza (`permissionColor`) bajo los detalles de la consulta.
- **Llavero:** `security find-generic-password` dispara el aviso del llavero; `keychainPassword` cachea en memoria por sesión para que no pregunte en cada consulta. El editor lee contraseñas al cargar bases y schemas.
- **Log:** abierta desde Finder, la app no tiene stdout. `runGUI` manda `log` a `~/.dbwatch/app.log`.
- **pgx:** `QueryExecModeSimpleProtocol` permite leer todo como texto con `RawValues()`. Los parámetros se interpolan del lado del cliente con escape seguro; los identificadores van con `pgx.Identifier{...}.Sanitize()`.
- **CSV:** el nombre lleva milisegundos; con solo segundos, dos ejecuciones manuales seguidas se pisaban.
- **`LSUIElement`:** la app no aparece en el Dock. Para traerla al frente desde una notificación, el delegado llama `[NSApp activateIgnoringOtherApps:YES]`.

## Ideas pendientes (conversadas, no implementadas)

- Avisos por Slack u otros canales además de macOS.
- Copiar una celda o fila desde el visor.
- Ejecutar una consulta sobre varios schemas a la vez (el usuario prefirió escribir consultas multi-schema).
