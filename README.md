# goHeadache

Command Line tool for checking Low Pressure Headache information in Japan

## Environment

Go 1.23.3

## Installation

```bash
cd goHeadache
go build
```

## Development Guidelines

- Do not run or build after each edit. The maintainer will run/build the code themselves.

## Usage

You can run goHeadache in two ways:

```bash
goHeadache <area_code> [-day <day>]
```

### Options

- `-day`: Filter output by specific day
  - Valid values: `yesterday`, `today`, `tomorrow`, `dayafter`
  - Optional: if omitted, shows all days

### Area Codes

Find your area code at: https://geoshape.ex.nii.ac.jp/ka/resource/

## Examples

For `Chiyoda, Tokyo` (area code: 13101):

```bash
# Show all days
$ goHeadache 13101

# Show only tomorrow's forecast
$ goHeadache 13101 -day tomorrow
```

Sample output:
```
Weather Report for 千代田区
--------------------------------------------

Today:
Time    Weather    Temp (°C)    Pressure (hPa)    Pressure Level
9:00    Sunny      18           1012              0
12:00   Sunny      22           1010              1
15:00   Cloudy     21           1008              2
...
```

## Data Source Credits

Weather data provided by:
- https://zutool.jp

Area codes sourced from:
- https://geoshape.ex.nii.ac.jp/ka/resource/