goHeadache

Command Line tool for checking Low Pressure Headache information in Japan

# Environment

Go 1.23.3

# Usage

```bash
cd goHeadache
go build
goHeadache <area_code>
```

See area codes at https://geoshape.ex.nii.ac.jp/ka/resource/

# Example

For `Chiyoda, Tokyo`, you can run

```bash
$ goHeadache 13101
```

# Credits

Weather data source:
- https://zutool.jp

Area codes:
- https://geoshape.ex.nii.ac.jp/ka/resource/