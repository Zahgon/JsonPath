Jayway JsonPath for Go
======================

**A Go library and command for reading and updating JSON documents with JsonPath expressions.**

This is a port of [Jayway JsonPath](https://github.com/json-path/JsonPath) 2.9.0 to Go. It keeps Jayway's path
syntax, filters, functions, evaluation options and error messages, and works on the JSON model of json-smart, the
default JSON provider of the Java library: documents are parsed permissively and numbers keep json-smart's types.

The module is `github.com/json-path/JsonPath` (Go 1.25, standard library only):

| Package     | Contents                                                                         |
| :---------- | :------------------------------------------------------------------------------- |
| `jsonpath`  | compiling paths and filters, reading, the write operations and the options       |
| `smartjson` | the JSON model: json-smart's permissive parser, its value types and serialiser   |
| `cmd/jsonpath` | the `jsonpath` command                                                        |

Getting Started
---------------

```shell
go get github.com/json-path/JsonPath
go build -o bin/jsonpath ./cmd/jsonpath
go test ./...
```

JsonPath expressions always refer to a JSON structure in the same way as XPath expression are used in combination
with an XML document. The "root member object" in JsonPath is always referred to as `$` regardless if it is an
object or array.

JsonPath expressions can use the dot–notation

`$.store.book[0].title`

or the bracket–notation

`$['store']['book'][0]['title']`

Operators
---------

| Operator                  | Description                                                        |
| :------------------------ | :----------------------------------------------------------------- |
| `$`                       | The root element to query. This starts all path expressions.       |
| `@`                       | The current node being processed by a filter predicate.            |
| `*`                       | Wildcard. Available anywhere a name or numeric are required.       |
| `..`                      | Deep scan. Available anywhere a name is required.                  |
| `.<name>`                 | Dot-notated child                                                  |
| `['<name>' (, '<name>')]` | Bracket-notated child or children                                  |
| `[<number> (, <number>)]` | Array index or indexes                                             |
| `[start:end]`             | Array slice operator                                               |
| `[?(<expression>)]`       | Filter expression. Expression must evaluate to a boolean value.    |


Functions
---------

Functions can be invoked at the tail end of a path - the input to a function is the output of the path expression.
The function output is dictated by the function itself.

| Function    | Description                                                                          | Output type          |
|:------------|:-------------------------------------------------------------------------------------|:---------------------|
| `min()`     | Provides the min value of an array of numbers                                        | double (float64)     |
| `max()`     | Provides the max value of an array of numbers                                        | double (float64)     |
| `avg()`     | Provides the average value of an array of numbers                                    | double (float64)     |
| `stddev()`  | Provides the standard deviation value of an array of numbers                         | double (float64)     |
| `length()`  | Provides the length of an array                                                      | int (int32)          |
| `sum()`     | Provides the sum value of an array of numbers                                        | double (float64)     |
| `keys()`    | Provides the property keys (An alternative for terminal tilde `~`)                   | key view of the object |
| `concat(X)` | Provides a concatinated version of the path output with a new item                   | like input           |
| `append(X)` | add an item to the json path output array                                            | like input           |
| `first()`   | Provides the first item of an array                                                  | Depends on the array |
| `last()`    | Provides the last item of an array                                                   | Depends on the array |
| `index(X)`  | Provides the item of an array of index: X, if the X is negative, take from backwards | Depends on the array |

Filter Operators
-----------------

Filters are logical expressions used to filter arrays. A typical filter would be `[?(@.age > 18)]` where `@` represents the current item being processed. More complex filters can be created with logical operators `&&` and `||`. String literals must be enclosed by single or double quotes (`[?(@.color == 'blue')]` or `[?(@.color == "blue")]`).

| Operator                 | Description                                                           |
| :----------------------- | :-------------------------------------------------------------------- |
| `==`                     | left is equal to right (note that 1 is not equal to '1')              |
| `!=`                     | left is not equal to right                                            |
| `<`                      | left is less than right                                               |
| `<=`                     | left is less or equal to right                                        |
| `>`                      | left is greater than right                                            |
| `>=`                     | left is greater than or equal to right                                |
| `=~`                     | left matches regular expression  [?(@.name =~ /foo.*?/i)]             |
| `in`                     | left exists in right [?(@.size in ['S', 'M'])]                        |
| `nin`                    | left does not exists in right                                         |
| `subsetof`               | left is a subset of right [?(@.sizes subsetof ['S', 'M', 'L'])]       |
| `anyof`                  | left has an intersection with right [?(@.sizes anyof ['M', 'L'])]     |
| `noneof`                 | left has no intersection with right [?(@.sizes noneof ['M', 'L'])]    |
| `size`                   | size of left (array or string) should match right                     |
| `empty`                  | left (array or string) should be empty                                |


Path Examples
-------------

Given the json

```javascript
{
    "store": {
        "book": [
            {
                "category": "reference",
                "author": "Nigel Rees",
                "title": "Sayings of the Century",
                "price": 8.95
            },
            {
                "category": "fiction",
                "author": "Evelyn Waugh",
                "title": "Sword of Honour",
                "price": 12.99
            },
            {
                "category": "fiction",
                "author": "Herman Melville",
                "title": "Moby Dick",
                "isbn": "0-553-21311-3",
                "price": 8.99
            },
            {
                "category": "fiction",
                "author": "J. R. R. Tolkien",
                "title": "The Lord of the Rings",
                "isbn": "0-395-19395-8",
                "price": 22.99
            }
        ],
        "bicycle": {
            "color": "red",
            "price": 19.95
        }
    },
    "expensive": 10
}
```

| JsonPath                                       | Result |
|:-------------------------------------------------------------------| :----- |
| `$.store.book[*].author` | The authors of all books     |
| `$..author`                           | All authors                         |
| `$.store.*`                           | All things, both books and bicycles  |
| `$.store..price`                 | The price of everything         |
| `$..book[2]`                         | The third book                      |
| `$..book[-2]`                       | The second to last book            |
| `$..book[0,1]`                     | The first two books               |
| `$..book[:2]`                       | All books from index 0 (inclusive) until index 2 (exclusive) |
| `$..book[1:2]`                     | All books from index 1 (inclusive) until index 2 (exclusive) |
| `$..book[-2:]`                     | Last two books                   |
| `$..book[2:]`                     | All books from index 2 (inclusive) to last  |
| `$..book[?(@.isbn)]`                                                 | All books with an ISBN number         |
| `$.store.book[?(@.price < 10)]`                                      | All books in store cheaper than 10  |
| `$..book[?(@.price <= $['expensive'])]`                              | All books in store that are not "expensive"  |
| `$..book[?(@.author =~ /.*REES/i)]`                                  | All books matching regex (ignore case)  |
| `$..*`                                                               | Give me every thing
| `$..book.length()`                                                   | The number of books                      |


The jsonpath Command
--------------------

`jsonpath` runs one operation on one JSON document read from standard input:

```shell
jsonpath [--option NAME]... [--filter FILTER]... [--limit N] [--typed] COMMAND ARGS...
```

| Command                        | Prints                                                            |
| :----------------------------- | :---------------------------------------------------------------- |
| `read PATH`                    | the value the path selects                                        |
| `set PATH VALUE`               | the result of the update, then the document                       |
| `put PATH KEY VALUE`           | the result of the update, then the document                       |
| `add PATH VALUE`               | the result of the update, then the document                       |
| `delete PATH`                  | the result of the update, then the document                       |
| `renameKey PATH OLDKEY NEWKEY` | the result of the update, then the document                       |
| `compile PATH`                 | the normalised path and whether it is definite (no input)         |
| `filter FILTER`                | the normalised filter (no input)                                  |

`--option` enables an evaluation option, `--filter` supplies the filters of `[?]` placeholders in order, `--limit`
stops a read after N results and `--typed` prints values with their number types (`int:1`, `long:3000000000`,
`double:1.5`, `bigdec:0.1234567890123456789`, ...). Values are printed as compact JSON, one per line. Failures go
to standard error with exit status 1 (JsonPath errors and illegal arguments), 2 (usage) or 3 (anything else).

```shell
$ jsonpath read '$..book[?(@.price < 10)].title' < store.json
["Sayings of the Century","Moby Dick"]
$ jsonpath --option AS_PATH_LIST read '$..book[0,1].price' < store.json
["$['store']['book'][0]['price']","$['store']['book'][1]['price']"]
```

Reading a Document
------------------

```go
doc, err := jsonpath.Parse(storeJSON)
if err != nil {
	return err
}
path, err := jsonpath.Compile("$.store.book[*].author")
if err != nil {
	return err
}
authors, err := path.Read(doc, jsonpath.DefaultConfiguration())
```

A `DocumentContext` keeps a parsed document and its configuration:

```go
ctx, err := jsonpath.ParseDocument(storeJSON, jsonpath.NewConfiguration(jsonpath.SuppressExceptions))
if err != nil {
	return err
}
cheap, err := ctx.Read("$.store.book[?(@.price < 10)]")
first, err := ctx.Limit(1).Read("$..price")
```

What is Returned When?
----------------------

A path is *definite* when it can select at most one value: it has no deep scan (`..`), wildcard, filter, slice or
list of indexes. A definite path returns the value it selects, an indefinite path returns an array of values (a
`*smartjson.Array`). Values are the types of package `smartjson`: `nil`, `bool`, `string`, `int32`, `int64`,
`*big.Int`, `float64`, `float32` (NaN only), `smartjson.BigDecimal`, `*smartjson.Object` and `*smartjson.Array`.
Integers take the smallest of int32, int64 and big.Int that holds them; decimals are float64, or `BigDecimal` when
their text is longer than 18 characters. Functions return float64 (`min`, `max`, `avg`, `stddev`, `sum`), int32
(`length`) or the selected item.

Predicates
----------

Filters are written inline, `[?(@.price < 10)]`, or passed to a `[?]` placeholder:

```go
cheap, err := jsonpath.ParseFilter("[?(@.price < 10)]")
if err != nil {
	return err
}
titles, err := ctx.Read("$.store.book[?].title", cheap)
```

`Filter.String` returns the normalised filter, as `[?(@['price'] < 10)]`. Any type implementing `Predicate` can be
used for a placeholder.

Set a value
-----------

```go
path, err := jsonpath.Compile("$['store']['book'][0]['author']")
if err != nil {
	return err
}
_, err = path.Set(doc, "Paul", jsonpath.DefaultConfiguration())
```

`Delete`, `Add` (to arrays), `Put` (a key into objects) and `RenameKey` work the same way. They change the document
in place and return it, or the changed paths with `AsPathList`.

Options
-------

| Option                  | Effect                                                                                  |
| :---------------------- | :-------------------------------------------------------------------------------------- |
| `DefaultPathLeafToNull` | a missing leaf reads as `null`                                                          |
| `AlwaysReturnList`      | a definite path returns its result in an array                                          |
| `AsPathList`            | the normalised paths of the results are returned instead of the values                  |
| `SuppressExceptions`    | a missing path returns `null` (definite) or an empty array instead of an error          |
| `RequireProperties`     | a missing property is an error even when the path is indefinite                         |

Errors
------

Errors are `*jsonpath.Error` values. `errors.Is` matches them against `ErrJSONPath`, `ErrInvalidPath`,
`ErrPathNotFound` (also an invalid path), `ErrInvalidJSON`, `ErrInvalidModification` and `ErrIllegalArgument`;
`Error()` gives Jayway's exception name and message, as
`PathNotFoundException: No results for path: $['store']['nope']`.
