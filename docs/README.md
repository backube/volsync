# Documentation

This documentation will be hosted at
[readthedocs](https://readthedocs.io).

## New to reStructuredText?

The following are some docs to help you get started:

* [reStructuredText
  primer](https://www.sphinx-doc.org/en/master/usage/restructuredtext/basics.html)
* [Quick reference](https://docutils.sourceforge.io/docs/user/rst/quickref.html)

## Building locally

To build and edit the docs locally, setup the python virtualenv and use the
Makefile:

```console
$ . ./setup-env.sh
.....
$ make html
Running Sphinx v3.0.4
loading pickled environment... done
building [mo]: targets for 0 po files that are out of date
building [html]: targets for 0 source files that are out of date
updating environment: 0 added, 0 changed, 0 removed
looking for now-outdated files... none found
no targets are out of date.
build succeeded.

The HTML pages are in _build/html.
```

You can then navigate to the root of the documentation at
`docs/_build/html/index.html`.
