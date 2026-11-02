# Перенос на GitHub

Чистая копия проекта и ZIP создаются командой:

```bash
python3 scripts/prepare_github.py
```

Результат: `dist/github/mapartscript/` и `dist/github/mapartscript-github.zip`.
Рядом находятся SHA-256 архива и исходных файлов. Скрипт использует `.gitignore`,
проверяет известные форматы ключей и токенов, сохраняет исходные локальные файлы.
Если папка экспорта уже существует, перенесите её перед повторным запуском:
скрипт не перезаписывает её, чтобы сохранить возможные изменения.

В копию входят исходники Go, тесты, данные каталога блоков, инструкции, лицензии,
Windows manifest/resource и GitHub Actions. Личные картинки, галереи проверок,
результаты экспорта, готовые бинарники и локальные отчёты не включаются.
Обычные тесты работают без скачанных картинок. В CLI-примерах `source.png`
означает ваш собственный файл; он не поставляется с проектом.

## Загрузка через Git

Целевой репозиторий — [andrey279/mapart](https://github.com/andrey279/mapart).
Для первого импорта он должен быть пустым, без добавления
README, `.gitignore` и лицензии. Откройте терминал в чистой копии:

```bash
cd dist/github/mapartscript
git init -b main
git add .
git diff --cached --stat
git commit -m "Initial import"
```

Если Git попросит имя и email, задайте их для этого репозитория через
`git config user.name` и `git config user.email`; можно использовать noreply-адрес
из настроек GitHub. Затем используйте адрес целевого репозитория (HTTPS требует локальной авторизации):

```bash
git remote add origin https://github.com/andrey279/mapart.git
git push -u origin main
```

Если репозиторий уже содержит коммиты, сначала клонируйте его, перенесите подготовленные
файлы в клон и создайте обычный коммит. Не заменяйте историю через force push.

Не используйте `git add -f`: исключённые локальные файлы не нужны для сборки.
Вместо Git можно распаковать ZIP и загрузить содержимое папки `mapartscript`
через **Add file → Upload files**, сохранив скрытые `.github` и `.gitignore`.
Не загружайте сам ZIP как единственный файл репозитория.

## Проверки и релизы

CI запускает тесты и `go vet` на Linux, Windows и macOS, проверяет Linux GUI
и собирает пакеты CLI для Linux/macOS, Windows с нативным GUI и Linux Fyne GUI.
Версия Go берётся из `go.mod`. Дополнительные секреты Codecov или Docker Hub
не нужны. Работоспособность GitHub Actions окончательно проверяется первым
запуском на GitHub; локальная проверка не запускает Windows/macOS тесты.

После успешного CI можно создать тег, выбрав свою версию:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Workflow создаст **черновик релиза** с ZIP-пакетами, лицензиями и SHA-256.
Откройте **Releases**, проверьте описание и опубликуйте черновик.
Сборка Linux GUI требует системных графических библиотек и при запуске.
Docker-образ собирается отдельно: `docker build -t mapartscript .`.

Справка: [actions/setup-go](https://github.com/actions/setup-go),
[GitHub CLI: создание релиза](https://cli.github.com/manual/gh_release_create).
