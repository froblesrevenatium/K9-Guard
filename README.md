# K9 Guard (dbwatch)

App de macOS que ejecuta consultas **de solo lectura** sobre tus conexiones PostgreSQL de DataGrip (local, stage y producción) con un horario, guarda cada resultado, dibuja su evolución y te avisa con una notificación cuando se cumple una regla.

> «Tú eliges dónde mirar; yo lo vigilo por ti.»

Documentación técnica para modificar la app (arquitectura, convenciones, trampas de Fyne y macOS): [`CLAUDE.md`](CLAUDE.md).

## Instalar y abrir

```bash
./build-app.sh                     # compila y crea ~/Applications/K9 Guard.app
open "$HOME/Applications/K9 Guard.app"
```

- Requiere Go 1.27+ (`brew install go`; el `go.mod` declara 1.27.3) y las Command Line Tools de Xcode (Fyne usa cgo).
- La app vive en la barra de menú (escudo blanco; rojo si hay alertas pendientes) y no ocupa lugar en el Dock. Cerrar la ventana solo la oculta.
- Para que arranque al iniciar sesión: `"$HOME/Applications/K9 Guard.app/Contents/MacOS/dbwatch" install-agent` (`uninstall-agent` lo quita).
- La primera vez macOS pide permiso para que K9 Guard mande notificaciones. Al hacer clic en una notificación se abre la consulta que la provocó, en su gráfica.

## Conexiones

- Se toman de DataGrip: todos los proyectos de `~/DataGripProjects/*`, las carpetas de proyecto que agregues y las conexiones globales de DataGrip.
- El engrane junto a **Consultas** abre **Fuentes de conexiones**: marca qué proyectos usar o agrega una carpeta de proyecto. Sin configurar, se usan todas.
- Las contraseñas se leen del llavero de macOS una vez por conexión y por sesión. Si macOS pregunta, elige **Permitir siempre**: el permiso queda para la herramienta `security` del sistema y no se pierde al recompilar.
- La primera lectura de cada contraseña puede mostrar el aviso del llavero. En DataGrip deben guardarse en *Settings → Appearance & Behavior → System Settings → Passwords → In native Keychain*.
- Los túneles SSH de DataGrip se abren solos al ejecutar y se cierran al terminar.
- **Permisos:** si tu usuario no tiene permiso sobre el esquema elegido, o PostgreSQL responde con privilegios insuficientes (código 42501), la consulta falla y, debajo de la línea de la conexión, aparece una franja amarillo mostaza con «Permisos insuficientes: …» y el mensaje de PostgreSQL. Nunca se lee otro esquema en su lugar.
- **Seguridad:** toda consulta corre en una transacción de solo lectura (`default_transaction_read_only=on` + `READ ONLY`), con `statement_timeout` de 5 min, y solo se aceptan `SELECT` o `WITH`. Nunca puede modificar datos, ni en producción.

## Ventana principal

- **Izquierda:** la lista de consultas. Cada fila muestra su estado (✔ dentro de la regla, ⚠ cumple la regla, ✖ error, ◉ alerta vista), y debajo, en gris, las filas y la fecha de la última ejecución. Tiene ▶ para ejecutarla al momento (se cambia por una **señal de advertencia mostaza** si la última ejecución tuvo errores o schemas omitidos) y un interruptor para activarla o desactivarla. Arriba está **Nueva consulta**. **Clic derecho en una consulta → Duplicar** crea una copia con el nombre «… (copia)» («… (copia 2)», etc. si ya existe), desactivada para que no notifique dos veces, y la selecciona. Abajo está **Eliminar**: arrastra una consulta encima, o selecciónala y haz clic; pide confirmación y conserva sus CSV.
- **Derecha:** sin consulta seleccionada se muestra la bienvenida. Con una consulta se muestran su nombre, **Enterado** (✓ verde, solo si hay alerta) y **Editar** (lápiz azul), y dos pestañas:
  - **Gráfica:** filas por fecha de ejecución, con la línea del umbral, en naranja los puntos que cumplieron la regla y en rojo los errores. Al pasar el mouse por un punto ves su fecha y cantidad. Arriba hay un resumen con mínimo, máximo y promedio.
  - **Detalle:** el historial de ejecuciones (un CSV por ejecución, se conservan 50) con **Limpiar** a la derecha, que tras confirmar borra los CSV, el historial de la gráfica y el último estado de la consulta (la consulta se conserva), un buscador que filtra filas por texto en cualquier columna y la tabla. La tabla (también la de Probar) empieza siempre con la columna **#**, el número consecutivo de cada fila en el resultado, alineado a la derecha, en negritas y con el fondo del encabezado, con el ancho justo para sus dígitos y anclada al desplazarse a la derecha. Los números con decimales se muestran cortados (sin redondear) a 4 decimales: `-2217.9999999` se ve `-2217.9999`; el CSV conserva el valor completo. Todo valor numérico se alinea a la derecha. Clic (o doble clic) en cualquier celda resalta toda su fila; al filtrar, cada fila conserva su número. Solo se muestra: no se guarda en el CSV. Las columnas toman un ancho inicial según su contenido y se pueden **redimensionar arrastrando el borde del encabezado**, que queda fijo al desplazarse. **Clic derecho en una celda → Copiar** copia su texto al portapapeles y muestra el aviso «Copiado: …» (también en la tabla de Probar).
- Menú de la barra: **Abrir K9 Guard** (va a la bienvenida), cada consulta (Enterado, Ejecutar ahora, Ver resultados, Editar), **Pausar horario** y **Salir**.

## Crear o editar una consulta

El formulario se abre como popup dentro de la ventana. Lo cierran 💾 Guardar, ✕ Cancelar o `Esc`; 🐞 **Probar** corre el formulario sin guardar ni notificar y muestra la tabla.

| Campo | Qué es |
|---|---|
| Nombre | Identificador de la consulta (también nombra su carpeta de resultados). |
| Origen | Conexión, Base de datos y Schema; las listas se cargan de la conexión elegida. El schema se usa como `search_path` y como `@schema`. En Schema también están **— todos los schemas —** y **— elegir varios… —** (ver Multiesquema). |
| SQL | Solo `SELECT`/`WITH`. Parámetros con `@Nombre`. |
| Parámetros | Uno por línea: `@MyParametro = 3`. |
| Horario | `4.5` = cada 4.5 h; `18:30` o `08:00, 18:30` = todos los días a esas horas; vacío = solo manual. |
| Notificaciones | `Si [operador] [cantidad]` (`>`, `<`, `!=`, `<>`, `>=`, `<=`) y **Solo si hay cambios**. |
| Máx. filas | Tope de filas que se guardan y muestran (por defecto 50 000). La regla siempre usa el total real. En multiesquema aparece **Máximo por schema**: sin marcar, el tope es sobre el total (los primeros schemas pueden llenarlo); marcado, cada schema guarda hasta ese máximo. |

### Parámetros (estilo T-SQL)

```sql
SELECT
    m.id,
    m.amount
FROM @schema.mastertransaction AS m
WHERE
    m.transactiontype = @MyParametro
    AND m.createddate > @since
```

```
@MyParametro = 3
@since = now-24h
```

- El tipo se deduce del valor: entero o decimal, `true`/`false`, fecha (`2026-10-20`, `2026-10-20 13:00:00`, `now`, `now-24h`, `today-7d`, también con `m`, `h`, `d`, `w`) o `'texto'` entre comillas simples.
- Si el parámetro va seguido de un punto (`@schema.tabla`), se cita como identificador; los demás viajan como parámetros de la consulta, sin riesgo de inyección.
- Se ignoran las `@` dentro de cadenas, comentarios y operadores como `@>`.

### Multiesquema

Con varios schemas (o todos), la consulta corre una vez por schema dentro de la misma transacción de solo lectura, cada uno aislado con un savepoint y con `@schema`/`search_path` apuntando a él. Los resultados se juntan con una primera columna `schema` y la regla se evalúa sobre el total. Los schemas sin permiso o donde la consulta falla (por ejemplo, porque falta una tabla) se omiten sin romper los demás y aparecen en la franja mostaza («Permisos insuficientes en N schemas…», «Errores en N schemas…»). Solo si fallan todos la consulta cuenta como error. «Todos» excluye `public`, `information_schema` y `pg_*`. Terminal: `--schemas "a,b,c"` o `--schemas "*"`.

### Reglas y alertas

- Se notifica cuando la cantidad de filas cumple la regla o cuando la consulta falla.
- **Solo si hay cambios:** solo notifica cuando la cantidad es distinta a la de la ejecución anterior. Sirve para consultas con fechas relativas que siempre traen la misma cantidad.
- La alerta queda pendiente hasta que marcas **Enterado**. La siguiente ejecución vuelve a evaluar la regla.
- **Horario:** si la app estuvo cerrada a la hora programada, se pone al día al abrirla. Una consulta nueva espera la siguiente hora programada; ▶ la corre al momento.

## Terminal

El mismo binario funciona como CLI (`dbwatch` en la carpeta del proyecto, o el ejecutable dentro de la app):

```bash
./dbwatch connections
./dbwatch add --name ajustes --connection Production-CRS --database booking --schema granhotelconcordia \
  --sql-file ajustes-duplicados.sql --every "08:00, 18:30" --operator ">" --threshold 0 --on-change \
  --param "@since = now-24h" --max-rows 20000
./dbwatch run ajustes --set "@since=2026-10-01"
./dbwatch list | show NOMBRE | enable NOMBRE | disable NOMBRE | seen NOMBRE | remove NOMBRE
./dbwatch ui | tray | serve | notify-test
```

## Archivos

Todo en `~/.dbwatch/` (otra ruta: `DBWATCH_HOME`):

| Archivo | Contenido |
|---|---|
| `queries.json` | Consultas definidas. |
| `state.json` | Última ejecución de cada consulta: filas, error, alerta, visto. |
| `results/<consulta>/<fecha>.csv` + `.meta.json` | Resultado de cada ejecución y su total real (50 por consulta). |
| `history/<consulta>.jsonl` | Cantidad de filas por ejecución, para la gráfica (5000 por consulta). |
| `settings.json` | Fuentes de conexiones elegidas. |
| `app.log` | Log de la app (ejecuciones, notificaciones, errores). |

## Compartir con el equipo

Cada persona ve sus propias conexiones de DataGrip, con sus propias credenciales del llavero. La app va firmada de forma local (ad hoc): en otra Mac, la primera vez hay que abrirla con clic derecho → Abrir, o correr `xattr -dr com.apple.quarantine "K9 Guard.app"`.
