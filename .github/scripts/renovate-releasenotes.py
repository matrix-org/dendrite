#!/usr/bin/env python

import os
import sys
import typer

from git import Repo
from loguru import logger
from pathlib import Path

from ruamel.yaml import YAML
from ruamel.yaml.comments import CommentedMap
from ruamel.yaml.scalarstring import LiteralScalarString
from typing import List

app = typer.Typer(add_completion=False)


def _setup_logging(debug):
    """
    Setup the log formatter for this script
    """

    log_level = "INFO"
    if debug:
        log_level = "DEBUG"

    logger.remove()
    logger.add(
        sys.stdout,
        colorize=True,
        format="<level>{message}</level>",
        level=log_level,
    )


@app.command()
def main(
    chart_folders: List[Path] = typer.Argument(
        ..., help="Folders containing the chart to process"),
    check_branch: str = typer.Option(
        None, help="The branch to compare against."),
    chart_base_folder: Path = typer.Option(
        "charts", help="The base folder where the charts reside."),
    debug: bool = False,
):
    _setup_logging(debug)

    git_repository = Repo(search_parent_directories=True)

    if check_branch:
        logger.info(f"Trying to find branch {check_branch}...")
        branch = next(
            (ref for ref in git_repository.remotes.origin.refs if ref.name == check_branch),
            None
        )
    else:
        logger.info(f"Trying to determine default branch...")
        branch = next(
            (ref for ref in git_repository.remotes.origin.refs if ref.name == "origin/HEAD"),
            None
        )

    if not branch:
        logger.error(
            f"Could not find branch {check_branch} to compare against.")
        raise typer.Exit(1)

    logger.info(f"Comparing against branch {branch}")

    for chart_folder in chart_folders:
        chart_folder = chart_base_folder.joinpath(chart_folder)
        if not chart_folder.is_dir():
            logger.error(f"Could not find folder {str(chart_folder)}")
            raise typer.Exit(1)

        chart_metadata_file = chart_folder.joinpath('Chart.yaml')

        if not chart_metadata_file.is_file():
            logger.error(f"Could not find file {str(chart_metadata_file)}")
            raise typer.Exit(1)

        logger.info(f"Updating changelog annotation for chart {chart_folder}")

        yaml = YAML(typ=['rt', 'string'])
        yaml.indent(mapping=2, sequence=4, offset=2)
        yaml.explicit_start = True
        yaml.preserve_quotes = True
        yaml.width = 4096

        old_chart_metadata = yaml.load(
            git_repository.git.show(f"{branch}:{chart_metadata_file}")
        )
        new_chart_metadata = yaml.load(chart_metadata_file.read_text())

        try:
            old_chart_dependencies = old_chart_metadata["dependencies"]
        except KeyError:
            old_chart_dependencies = []

        try:
            new_chart_dependencies = new_chart_metadata["dependencies"]
        except KeyError:
            new_chart_dependencies = []

        annotations = []
        for dependency in new_chart_dependencies:
            old_dep = None
            if "alias" in dependency.keys():
                old_dep = next(
                    (old_dep for old_dep in old_chart_dependencies if "alias" in old_dep.keys(
                    ) and old_dep["alias"] == dependency["alias"]),
                    None
                )
            else:
                old_dep = next(
                    (old_dep for old_dep in old_chart_dependencies if old_dep["name"] == dependency["name"]),
                    None
                )

            add_annotation = False
            if old_dep:
                if dependency["version"] != old_dep["version"]:
                    add_annotation = True
            else:
                add_annotation = True

            if add_annotation:
                if "alias" in dependency.keys():
                    annotations.append({
                        "kind": "changed",
                        "description": f"Upgraded `{dependency['name']}` chart dependency to version {dependency['version']} for alias '{dependency['alias']}'"
                    })
                else:
                    annotations.append({
                        "kind": "changed",
                        "description": f"Upgraded `{dependency['name']}` chart dependency to version {dependency['version']}"
                    })

        if annotations:
            annotations = YAML(typ=['rt', 'string']
                               ).dump_to_string(annotations)

            if not "annotations" in new_chart_metadata:
                new_chart_metadata["annotations"] = CommentedMap()

            new_chart_metadata["annotations"]["artifacthub.io/changes"] = LiteralScalarString(
                annotations)
            yaml.dump(new_chart_metadata, chart_metadata_file)


if __name__ == "__main__":
    app()
