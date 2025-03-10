# SPDX-FileCopyrightText: 2023-present deepset GmbH <info@deepset.ai>
#
# SPDX-License-Identifier: Apache-2.0
from datetime import datetime
from itertools import chain
from typing import Any, Dict, List, Literal, Tuple

from haystack.errors import FilterError
from psycopg.sql import SQL
from psycopg.types.json import Jsonb

# we need this mapping to cast meta values to the correct type,
# since they are stored in the JSONB field as strings.
# this dict can be extended if needed
PYTHON_TYPES_TO_PG_TYPES = {
    int: "integer",
    float: "real",
    bool: "boolean",
}

NO_VALUE = "no_value"


def _convert_filters_to_where_clause_and_params(
    filters: Dict[str, Any], operator: Literal["WHERE", "AND"] = "WHERE"
) -> Tuple[SQL, Tuple]:
    """
    Convert Haystack filters to a WHERE clause and a tuple of params to query PostgreSQL.
    """
    if "field" in filters:
        query, values = _parse_comparison_condition(filters)
    else:
        query, values = _parse_logical_condition(filters)

    where_clause = SQL(f" {operator} ") + SQL(query)
    params = tuple(value for value in values if value != NO_VALUE)

    return where_clause, params


def _parse_logical_condition(condition: Dict[str, Any]) -> Tuple[str, List[Any]]:
    if "operator" not in condition:
        msg = f"'operator' key missing in {condition}"
        raise FilterError(msg)
    if "conditions" not in condition:
        msg = f"'conditions' key missing in {condition}"
        raise FilterError(msg)

    operator = condition["operator"]
    if operator not in ["AND", "OR"]:
        msg = f"Unknown logical operator '{operator}'. Valid operators are: 'AND', 'OR'"
        raise FilterError(msg)

    # logical conditions can be nested, so we need to parse them recursively
    conditions = []
    for c in condition["conditions"]:
        if "field" in c:
            query, vals = _parse_comparison_condition(c)
        else:
            query, vals = _parse_logical_condition(c)
        conditions.append((query, vals))

    query_parts, values = [], []
    for c in conditions:
        query_parts.append(c[0])
        values.append(c[1])
    if isinstance(values[0], list):
        values = list(chain.from_iterable(values))

    if operator == "AND":
        sql_query = f"({' AND '.join(query_parts)})"
    elif operator == "OR":
        sql_query = f"({' OR '.join(query_parts)})"
    else:
        msg = f"Unknown logical operator '{operator}'"
        raise FilterError(msg)

    return sql_query, values


def _parse_comparison_condition(condition: Dict[str, Any]) -> Tuple[str, List[Any]]:
    field: str = condition["field"]
    if "operator" not in condition:
        msg = f"'operator' key missing in {condition}"
        raise FilterError(msg)
    if "value" not in condition:
        msg = f"'value' key missing in {condition}"
        raise FilterError(msg)
    operator: str = condition["operator"]
    if operator not in COMPARISON_OPERATORS:
        msg = f"Unknown comparison operator '{operator}'. Valid operators are: {list(COMPARISON_OPERATORS.keys())}"
        raise FilterError(msg)

    value: Any = condition["value"]

    if field.startswith("meta."):
        field = _treat_meta_field(field, value)

    field, value = COMPARISON_OPERATORS[operator](field, value)
    return field, [value]


def _treat_meta_field(field: str, value: Any) -> str:
    """
    Internal method that modifies the field str
    to make the meta JSONB field queryable.

    Examples:
    >>> _treat_meta_field(field="meta.number", value=9)
    "(meta->>'number')::integer"

    >>> _treat_meta_field(field="meta.name", value="my_name")
    "meta->>'name'"
    """

    # use the ->> operator to access keys in the meta JSONB field
    field_name = field.split(".", 1)[-1]
    field = f"meta->>'{field_name}'"

    # meta fields are stored as strings in the JSONB field,
    # so we need to cast them to the correct type
    type_value = PYTHON_TYPES_TO_PG_TYPES.get(type(value))
    if isinstance(value, list) and len(value) > 0:
        type_value = PYTHON_TYPES_TO_PG_TYPES.get(type(value[0]))

    if type_value:
        field = f"({field})::{type_value}"

    return field


def _equal(field: str, value: Any) -> Tuple[str, Any]:
    if value is None:
        # NO_VALUE is a placeholder that will be removed in _convert_filters_to_where_clause_and_params
        return f"{field} IS NULL", NO_VALUE
    return f"{field} = %s", value


def _not_equal(field: str, value: Any) -> Tuple[str, Any]:
    # we use IS DISTINCT FROM to correctly handle NULL values
    # (not handled by !=)
    return f"{field} IS DISTINCT FROM %s", value


def _greater_than(field: str, value: Any) -> Tuple[str, Any]:
    if isinstance(value, str):
        try:
            datetime.fromisoformat(value)
        except (ValueError, TypeError) as exc:
            msg = (
                "Can't compare strings using operators '>', '>=', '<', '<='. "
                "Strings are only comparable if they are ISO formatted dates."
            )
            raise FilterError(msg) from exc
    if type(value) in [list, Jsonb]:
        msg = f"Filter value can't be of type {type(value)} using operators '>', '>=', '<', '<='"
        raise FilterError(msg)

    return f"{field} > %s", value


def _greater_than_equal(field: str, value: Any) -> Tuple[str, Any]:
    if isinstance(value, str):
        try:
            datetime.fromisoformat(value)
        except (ValueError, TypeError) as exc:
            msg = (
                "Can't compare strings using operators '>', '>=', '<', '<='. "
                "Strings are only comparable if they are ISO formatted dates."
            )
            raise FilterError(msg) from exc
    if type(value) in [list, Jsonb]:
        msg = f"Filter value can't be of type {type(value)} using operators '>', '>=', '<', '<='"
        raise FilterError(msg)

    return f"{field} >= %s", value


def _less_than(field: str, value: Any) -> Tuple[str, Any]:
    if isinstance(value, str):
        try:
            datetime.fromisoformat(value)
        except (ValueError, TypeError) as exc:
            msg = (
                "Can't compare strings using operators '>', '>=', '<', '<='. "
                "Strings are only comparable if they are ISO formatted dates."
            )
            raise FilterError(msg) from exc
    if type(value) in [list, Jsonb]:
        msg = f"Filter value can't be of type {type(value)} using operators '>', '>=', '<', '<='"
        raise FilterError(msg)

    return f"{field} < %s", value


def _less_than_equal(field: str, value: Any) -> Tuple[str, Any]:
    if isinstance(value, str):
        try:
            datetime.fromisoformat(value)
        except (ValueError, TypeError) as exc:
            msg = (
                "Can't compare strings using operators '>', '>=', '<', '<='. "
                "Strings are only comparable if they are ISO formatted dates."
            )
            raise FilterError(msg) from exc
    if type(value) in [list, Jsonb]:
        msg = f"Filter value can't be of type {type(value)} using operators '>', '>=', '<', '<='"
        raise FilterError(msg)

    return f"{field} <= %s", value


def _not_in(field: str, value: Any) -> Tuple[str, List]:
    if not isinstance(value, list):
        msg = f"{field}'s value must be a list when using 'not in' comparator in Pinecone"
        raise FilterError(msg)

    return f"{field} IS NULL OR {field} != ALL(%s)", [value]


def _in(field: str, value: Any) -> Tuple[str, List]:
    if not isinstance(value, list):
        msg = f"{field}'s value must be a list when using 'in' comparator in Pinecone"
        raise FilterError(msg)

    # see https://www.psycopg.org/psycopg3/docs/basic/adapt.html#lists-adaptation
    return f"{field} = ANY(%s)", [value]


def _like(field: str, value: Any) -> Tuple[str, Any]:
    if not isinstance(value, str):
        msg = f"{field}'s value must be a str when using 'LIKE' "
        raise FilterError(msg)
    return f"{field} LIKE %s", value


def _not_like(field: str, value: Any) -> Tuple[str, Any]:
    if not isinstance(value, str):
        msg = f"{field}'s value must be a str when using 'LIKE' "
        raise FilterError(msg)
    return f"{field} NOT LIKE %s", value


COMPARISON_OPERATORS = {
    "==": _equal,
    "!=": _not_equal,
    ">": _greater_than,
    ">=": _greater_than_equal,
    "<": _less_than,
    "<=": _less_than_equal,
    "in": _in,
    "not in": _not_in,
    "like": _like,
    "not like": _not_like,
}
