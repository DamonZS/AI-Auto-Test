"""控制面最小垂直切片：租户 -> 项目 -> 用例 -> 执行 -> 报告。"""

import uuid

from fastapi import Depends, FastAPI, HTTPException, Header
from pydantic import BaseModel

from .db import connect

app = FastAPI(title="测试平台控制面")


def tenant(x_tenant_id: str | None = Header(default=None)) -> str:
    if not x_tenant_id:
        raise HTTPException(400, "缺少 X-Tenant-Id")
    with connect() as conn:
        if not conn.execute("SELECT 1 FROM tenants WHERE id = ?", (x_tenant_id,)).fetchone():
            raise HTTPException(404, "租户不存在")
    return x_tenant_id


class TenantIn(BaseModel):
    name: str


class ProjectIn(BaseModel):
    name: str
    base_url: str


class CaseIn(BaseModel):
    project_id: str
    name: str
    scenario: str
    type: str = "ui"


@app.post("/api/tenants")
def create_tenant(body: TenantIn):
    tid = uuid.uuid4().hex[:12]
    with connect() as conn:
        conn.execute("INSERT INTO tenants (id, name) VALUES (?, ?)", (tid, body.name))
    return {"id": tid, "name": body.name}


@app.get("/api/tenants")
def list_tenants():
    with connect() as conn:
        return [dict(r) for r in conn.execute("SELECT * FROM tenants ORDER BY created_at")]


@app.post("/api/projects")
def create_project(body: ProjectIn, tid: str = Depends(tenant)):
    pid = uuid.uuid4().hex[:12]
    with connect() as conn:
        conn.execute(
            "INSERT INTO projects (id, tenant_id, name, base_url) VALUES (?, ?, ?, ?)",
            (pid, tid, body.name, body.base_url),
        )
    return {"id": pid, "name": body.name, "base_url": body.base_url}


@app.get("/api/projects")
def list_projects(tid: str = Depends(tenant)):
    with connect() as conn:
        return [dict(r) for r in conn.execute(
            "SELECT * FROM projects WHERE tenant_id = ? ORDER BY created_at", (tid,))]


@app.post("/api/cases")
def create_case(body: CaseIn, tid: str = Depends(tenant)):
    cid = uuid.uuid4().hex[:12]
    with connect() as conn:
        pid = conn.execute(
            "SELECT id FROM projects WHERE id = ? AND tenant_id = ?", (body.project_id, tid)
        ).fetchone()
        if not pid:
            raise HTTPException(404, "项目不存在")
        conn.execute(
            "INSERT INTO cases (id, tenant_id, project_id, name, type, scenario)"
            " VALUES (?, ?, ?, ?, ?, ?)",
            (cid, tid, body.project_id, body.name, body.type, body.scenario),
        )
    return {"id": cid, "name": body.name}


@app.get("/api/cases")
def list_cases(project_id: str | None = None, tid: str = Depends(tenant)):
    sql = "SELECT * FROM cases WHERE tenant_id = ?"
    args = [tid]
    if project_id:
        sql += " AND project_id = ?"
        args.append(project_id)
    with connect() as conn:
        return [dict(r) for r in conn.execute(sql + " ORDER BY created_at", args)]


@app.post("/api/cases/{case_id}/run")
def run_case(case_id: str, tid: str = Depends(tenant)):
    rid = uuid.uuid4().hex[:12]
    with connect() as conn:
        case = conn.execute(
            "SELECT * FROM cases WHERE id = ? AND tenant_id = ?", (case_id, tid)
        ).fetchone()
        if not case:
            raise HTTPException(404, "用例不存在")
        conn.execute(
            "INSERT INTO runs (id, tenant_id, case_id, status) VALUES (?, ?, ?, 'queued')",
            (rid, tid, case_id),
        )
    return {"run_id": rid, "status": "queued"}


@app.get("/api/runs/{run_id}")
def get_run(run_id: str, tid: str = Depends(tenant)):
    with connect() as conn:
        run = conn.execute(
            "SELECT * FROM runs WHERE id = ? AND tenant_id = ?", (run_id, tid)
        ).fetchone()
    if not run:
        raise HTTPException(404, "执行记录不存在")
    return dict(run)


@app.get("/api/runs")
def list_runs(tid: str = Depends(tenant)):
    with connect() as conn:
        rows = conn.execute(
            "SELECT r.*, c.name AS case_name FROM runs r"
            " JOIN cases c ON c.id = r.case_id"
            " WHERE r.tenant_id = ? ORDER BY r.started_at DESC LIMIT 50",
            (tid,),
        )
        return [dict(r) for r in rows]
