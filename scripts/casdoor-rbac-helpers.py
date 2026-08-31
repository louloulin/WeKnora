#!/usr/bin/env python3
"""Helper functions for setup-casdoor-rbac.sh — keeps complex JSON building out of bash."""
import json, os, sys

def perm_payload(owner, name, actions_str, resources, **extra):
    actions = [a.strip() for a in actions_str.split(",")]
    return json.dumps({
        "owner": owner,
        "name": name,
        "displayName": name,
        "description": "Unified WeKnora permission",
        "resources": [resources],
        "actions": actions,
        "effect": "Allow",
        "isEnabled": True,
    })

def role_payload(owner, name, display, description):
    return json.dumps({
        "owner": owner,
        "name": name,
        "displayName": display,
        "description": description,
        "isEnabled": True,
    })

def model_payload(owner, name, text):
    return json.dumps({
        "owner": owner,
        "name": name,
        "displayName": "WeKnora RBAC Model (sub, obj, act)",
        "description": "3-tuple RBAC with role inheritance. p.act must match exactly (no wildcard privilege escalation).",
        "text": text,
    })

def adapter_payload(owner, name, table):
    return json.dumps({
        "owner": owner,
        "name": name,
        "table": table,
        "useSameDb": True,
    })

def enforcer_payload(owner, name, display, model_id, adapter_id):
    return json.dumps({
        "owner": owner,
        "name": name,
        "displayName": display,
        "model": model_id,
        "adapter": adapter_id,
    })

def policy_payload(ptype, v0, v1="", v2=""):
    d = {"Ptype": ptype, "V0": v0, "V1": v1, "V2": v2, "V3": "", "V4": "", "V5": ""}
    return json.dumps(d)

def check_name_in_list(items, name):
    for p in items or []:
        if isinstance(p, dict) and p.get("name") == name:
            return True
    return False

if __name__ == "__main__":
    cmd = sys.argv[1] if len(sys.argv) > 1 else ""
    if cmd == "perm-payload":
        print(perm_payload(sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5]))
    elif cmd == "role-payload":
        print(role_payload(sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5]))
    elif cmd == "enforcer-payload":
        print(enforcer_payload(sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6]))
    elif cmd == "adapter-payload":
        print(adapter_payload(sys.argv[2], sys.argv[3], sys.argv[4]))
    elif cmd == "policy-payload":
        args = sys.argv[2:]
        print(policy_payload(*args[:4]))
    else:
        sys.exit(1)
