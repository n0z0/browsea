# browsea
A Corporate Browser


Build
```powershell
go build -ldflags "-s -w" -o browsea.exe
```

Build tanpa cmd (Background / Hidden Console)
```powershell
go build -ldflags "-s -w -H windowsgui" -o browsea.exe
```

Run
```powershell
./browsea.exe
```

```powershell
git tag v0.0.2
git push origin --tags
go list -m github.com/n0z0/browsea@v0.0.2
```