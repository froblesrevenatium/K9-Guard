# K9 Guard

App de macOS que vigila tus consultas de solo lectura en las bases de datos de DataGrip y te avisa con una notificación cuando se cumple una regla.

> «Tú eliges dónde mirar; yo lo vigilo por ti.»

## Instalar

1. Instala Go y las herramientas de Xcode (solo la primera vez):

   ```bash
   brew install go
   xcode-select --install
   ```

2. Descarga el proyecto y compila la app:

   ```bash
   git clone https://github.com/froblesrevenatium/K9-Guard.git
   cd K9-Guard
   ./build-app.sh
   ```

3. Ábrela:

   ```bash
   open "$HOME/Applications/K9 Guard.app"
   ```

La app vive en la barra de menú (escudo blanco). La primera vez macOS te pedirá permiso para mandar notificaciones y para leer las contraseñas de DataGrip en el llavero; elige **Permitir siempre**.

Para que arranque al iniciar sesión:

```bash
"$HOME/Applications/K9 Guard.app/Contents/MacOS/dbwatch" install-agent
```

## Más información

- [MANUAL.md](MANUAL.md): cómo crear consultas, reglas, horarios y todo lo demás.
