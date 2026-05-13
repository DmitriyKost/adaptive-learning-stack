-- Demo dataset matching seed tasks in task-progress-service.
-- Data is stored in read-only task_data schema. User workspaces remain isolated.

CREATE TABLE IF NOT EXISTS task_data.employees (
    id BIGINT PRIMARY KEY,
    name TEXT NOT NULL,
    department TEXT NOT NULL,
    salary INT NOT NULL,
    hired_at DATE NOT NULL
);

CREATE TABLE IF NOT EXISTS task_data.customers (
    id BIGINT PRIMARY KEY,
    name TEXT NOT NULL,
    city TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS task_data.orders (
    id BIGINT PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES task_data.customers(id),
    total NUMERIC(12, 2) NOT NULL,
    created_at DATE NOT NULL
);

TRUNCATE task_data.orders, task_data.customers, task_data.employees RESTART IDENTITY;

INSERT INTO task_data.employees (id, name, department, salary, hired_at) VALUES
    (1, 'Ivan',  'Engineering', 180000, '2022-01-10'),
    (2, 'Anna',  'Engineering', 210000, '2021-09-15'),
    (3, 'Petr',  'Analytics',   160000, '2023-03-20'),
    (4, 'Maria', 'Education',   150000, '2020-11-05'),
    (5, 'Olga',  'Analytics',   125000, '2022-07-11'),
    (6, 'Sergey','Support',      90000, '2024-02-01');

INSERT INTO task_data.customers (id, name, city) VALUES
    (1, 'Acme Corp',     'Moscow'),
    (2, 'DataSoft',      'Saint Petersburg'),
    (3, 'EduLab',        'Kazan'),
    (4, 'NoOrders Ltd',  'Novosibirsk');

INSERT INTO task_data.orders (id, customer_id, total, created_at) VALUES
    (1, 1,  4200.00, '2024-01-10'),
    (2, 1,  7600.00, '2024-02-14'),
    (3, 2, 12500.00, '2024-03-03'),
    (4, 3,  2100.00, '2024-04-22'),
    (5, 2,  5800.00, '2024-05-17');

GRANT SELECT ON ALL TABLES IN SCHEMA task_data TO playground_readonly;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA task_data TO playground_readonly;
