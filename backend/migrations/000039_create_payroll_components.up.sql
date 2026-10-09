CREATE TABLE payroll_components (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    structure_id UUID NOT NULL REFERENCES payroll_structures(id) ON DELETE CASCADE,
    code VARCHAR(20) NOT NULL CHECK (code ~ '^[A-Z0-9_]{2,20}$'),
    name VARCHAR(100) NOT NULL,
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('earning', 'deduction')),
    calc VARCHAR(20) NOT NULL CHECK (calc IN ('fixed', 'percent_of_ctc', 'percent_of_basic', 'balance')),
    value NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (value >= 0),
    taxable BOOLEAN NOT NULL DEFAULT TRUE,
    position INT NOT NULL
);

-- PY-015: components are immutable; codes unique within a structure.
CREATE UNIQUE INDEX uq_payroll_components_structure_code ON payroll_components (structure_id, code);
