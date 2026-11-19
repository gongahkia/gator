#!/usr/bin/env python3
"""Bake the exact, trusted A1 USDA mesh records; not a general USD parser."""
import hashlib
import json
import math
from pathlib import Path
import re

root = Path(__file__).resolve().parent
source = root.parent / "validation/Assets/Gator-B.usda"
stack, nodes, mesh = [], [], None
def numbers(text):
    return [float(x) for x in re.findall(r"[-+]?(?:\d*\.\d+|\d+)(?:[eE][-+]?\d+)?", text)]
for line in source.read_text().splitlines():
    indent = len(line) - len(line.lstrip())
    match = re.search(r'def Xform "([^"]+)"', line)
    if match:
        while stack and stack[-1][0] >= indent:
            stack.pop()
        node = dict(name=match[1], translation=[0,0,0], rotation=[0,0,0], scale=[1,1,1], visible=True, children=[])
        (stack[-1][1]["children"] if stack else nodes).append(node)
        stack.append((indent, node)); mesh = None
    if not stack:
        continue
    node = stack[-1][1]
    if 'def Mesh "Surface"' in line:
        mesh = dict(positions=[], normals=[], indices=[], color=[0,0,0], emission=[0,0,0], roughness=0.72, metallic=0)
        node["mesh"] = mesh
    for field, token in [("translation", "xformOp:translate"), ("rotation", "xformOp:rotateXYZ"), ("scale", "xformOp:scale")]:
        if token + " =" in line:
            node[field] = numbers(line.split("=", 1)[1])
    if 'token visibility = "invisible"' in line:
        node["visible"] = False
    if mesh is not None:
        for field, token in [("positions", "point3f[] points"), ("normals", "normal3f[] normals"), ("indices", "int[] faceVertexIndices")]:
            if token + " =" in line:
                data = numbers(line.split("[",2)[-1].split("]",1)[0])
                mesh[field] = [int(x) for x in data] if field == "indices" else [data[i:i+3] for i in range(0,len(data),3)]
        for field, token in [("color","inputs:diffuseColor"),("emission","inputs:emissiveColor"),("roughness","inputs:roughness"),("metallic","inputs:metallic")]:
            if token + " =" in line:
                data = numbers(line.split("=",1)[1]);mesh[field] = data if field in ["color","emission"] else data[0]

counts = dict(meshes=0, vertices=0, triangles=0)
def validate(node):
    assert len(node["translation"]) == len(node["rotation"]) == len(node["scale"]) == 3
    if "mesh" in node:
        mesh=node["mesh"];assert len(mesh["positions"]) == len(mesh["normals"]) > 0
        assert len(mesh["indices"]) % 3 == 0 and max(mesh["indices"]) < len(mesh["positions"]) and min(mesh["indices"]) >= 0
        assert all(len(p)==3 and all(math.isfinite(v) for v in p) for p in mesh["positions"]+mesh["normals"])
        counts["meshes"]+=1;counts["vertices"]+=len(mesh["positions"]);counts["triangles"]+=len(mesh["indices"])//3
    for child in node["children"]:validate(child)
assert len(nodes)==1 and nodes[0]["name"]=="Gator_B"
validate(nodes[0]);assert counts==dict(meshes=26,vertices=8609,triangles=14164),counts
document=dict(version=1,sourceSHA256=hashlib.sha256(source.read_bytes()).hexdigest(),counts=counts,root=nodes[0])
destination=root/"Assets/B-native-meshes.json"
destination.parent.mkdir(exist_ok=True)
destination.write_text(json.dumps(document,separators=(",",":"),allow_nan=False))
print(json.dumps(dict(file=str(destination),bytes=destination.stat().st_size,sha256=hashlib.sha256(destination.read_bytes()).hexdigest(),**counts)))
