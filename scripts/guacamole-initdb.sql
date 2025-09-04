--
-- Licensed to the Apache Software Foundation (ASF) under one
-- or more contributor license agreements.  See the NOTICE file
-- distributed with this work for additional information
-- regarding copyright ownership.  The ASF licenses this file
-- to you under the Apache License, Version 2.0 (the
-- "License"); you may not use this file except in compliance
-- with the License.  You may obtain a copy of the License at
--
--   http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing,
-- software distributed under the License is distributed on an
-- "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
-- KIND, either express or implied.  See the License for the
-- specific language governing permissions and limitations
-- under the License.
--

-- Create Guacamole database schema

--
-- Table of connection groups. Each connection group has a name.
--

CREATE TABLE guacamole_connection_group (

  connection_group_id   SERIAL       NOT NULL,
  parent_id             INTEGER,
  connection_group_name VARCHAR(128) NOT NULL,
  type                  VARCHAR(32)  NOT NULL
                        DEFAULT 'ORGANIZATIONAL',

  -- Concurrency limits
  max_connections          INTEGER,
  max_connections_per_user INTEGER,
  enable_session_affinity  BOOLEAN NOT NULL DEFAULT FALSE,

  PRIMARY KEY (connection_group_id),

  CONSTRAINT guacamole_connection_group_name_parent
    UNIQUE (connection_group_name, parent_id),

  CONSTRAINT guacamole_connection_group_parent
    FOREIGN KEY (parent_id)
    REFERENCES guacamole_connection_group (connection_group_id) ON DELETE CASCADE

);

CREATE INDEX ON guacamole_connection_group(parent_id);

--
-- Table of connections. Each connection has a name and protocol.
--

CREATE TABLE guacamole_connection (

  connection_id       SERIAL       NOT NULL,
  connection_name     VARCHAR(128) NOT NULL,
  parent_id           INTEGER,
  protocol            VARCHAR(32)  NOT NULL,

  -- Concurrency limits
  max_connections          INTEGER,
  max_connections_per_user INTEGER,
  connection_weight        INTEGER,
  failover_only            BOOLEAN NOT NULL DEFAULT FALSE,

  PRIMARY KEY (connection_id),

  CONSTRAINT guacamole_connection_name_parent
    UNIQUE (connection_name, parent_id),

  CONSTRAINT guacamole_connection_parent
    FOREIGN KEY (parent_id)
    REFERENCES guacamole_connection_group (connection_group_id) ON DELETE CASCADE

);

CREATE INDEX ON guacamole_connection(parent_id);

--
-- Table of users. Each user has a unique username and a hashed password
-- with corresponding salt. Although the authentication system will always set
-- salted passwords, other systems may set unsalted passwords by simply not
-- providing the salt.
--

CREATE TABLE guacamole_entity (

  entity_id     SERIAL        NOT NULL,
  name          VARCHAR(128)  NOT NULL,
  type          VARCHAR(16)   NOT NULL,

  PRIMARY KEY (entity_id),

  CONSTRAINT guacamole_entity_name_scope
    UNIQUE (type, name)

);

CREATE TABLE guacamole_user (

  user_id       SERIAL        NOT NULL,
  entity_id     INTEGER       NOT NULL,

  -- Optionally-salted password
  password_hash BYTEA         NOT NULL,
  password_salt BYTEA,
  password_date timestamptz   NOT NULL DEFAULT CURRENT_TIMESTAMP,

  -- Account disabled/expired status
  disabled      BOOLEAN       NOT NULL DEFAULT FALSE,
  expired       BOOLEAN       NOT NULL DEFAULT FALSE,

  -- Time-based access restriction
  access_window_start    TIME,
  access_window_end      TIME,

  -- Date-based access restriction
  valid_from  DATE,
  valid_until DATE,

  -- Timezone used for all date/time comparisons and interpretation
  timezone VARCHAR(64),

  -- Profile information
  full_name           VARCHAR(256),
  email_address       VARCHAR(256),
  organization        VARCHAR(256),
  organizational_role VARCHAR(256),

  PRIMARY KEY (user_id),

  CONSTRAINT guacamole_user_single_entity
    UNIQUE (entity_id),

  CONSTRAINT guacamole_user_entity
    FOREIGN KEY (entity_id)
    REFERENCES guacamole_entity (entity_id)
    ON DELETE CASCADE

);

--
-- Table of user groups. Each user group may have an arbitrary set of member
-- users and member groups, with those members inheriting the permissions
-- granted to that group.
--

CREATE TABLE guacamole_user_group (

  user_group_id SERIAL        NOT NULL,
  entity_id     INTEGER       NOT NULL,

  -- Group disabled status
  disabled      BOOLEAN       NOT NULL DEFAULT FALSE,

  PRIMARY KEY (user_group_id),

  CONSTRAINT guacamole_user_group_single_entity
    UNIQUE (entity_id),

  CONSTRAINT guacamole_user_group_entity
    FOREIGN KEY (entity_id)
    REFERENCES guacamole_entity (entity_id)
    ON DELETE CASCADE

);

--
-- Connection parameters
--

CREATE TABLE guacamole_connection_parameter (

  connection_id   INTEGER       NOT NULL,
  parameter_name  VARCHAR(128)  NOT NULL,
  parameter_value VARCHAR(4096) NOT NULL,

  PRIMARY KEY (connection_id,parameter_name),

  CONSTRAINT guacamole_connection_parameter_ibfk_1
    FOREIGN KEY (connection_id)
    REFERENCES guacamole_connection (connection_id) ON DELETE CASCADE

);

CREATE INDEX ON guacamole_connection_parameter(connection_id);

--
-- Connection permissions
--

CREATE TABLE guacamole_connection_permission (

  entity_id     INTEGER NOT NULL,
  connection_id INTEGER NOT NULL,
  permission    VARCHAR(16) NOT NULL,

  PRIMARY KEY (entity_id,connection_id,permission),

  CONSTRAINT guacamole_connection_permission_ibfk_1
    FOREIGN KEY (connection_id)
    REFERENCES guacamole_connection (connection_id) ON DELETE CASCADE,

  CONSTRAINT guacamole_connection_permission_entity
    FOREIGN KEY (entity_id)
    REFERENCES guacamole_entity (entity_id) ON DELETE CASCADE

);

CREATE INDEX ON guacamole_connection_permission(connection_id);
CREATE INDEX ON guacamole_connection_permission(entity_id);

--
-- Connection group permissions
--

CREATE TABLE guacamole_connection_group_permission (

  entity_id           INTEGER NOT NULL,
  connection_group_id INTEGER NOT NULL,
  permission          VARCHAR(16) NOT NULL,

  PRIMARY KEY (entity_id,connection_group_id,permission),

  CONSTRAINT guacamole_connection_group_permission_ibfk_1
    FOREIGN KEY (connection_group_id)
    REFERENCES guacamole_connection_group (connection_group_id) ON DELETE CASCADE,

  CONSTRAINT guacamole_connection_group_permission_entity
    FOREIGN KEY (entity_id)
    REFERENCES guacamole_entity (entity_id) ON DELETE CASCADE

);

CREATE INDEX ON guacamole_connection_group_permission(connection_group_id);
CREATE INDEX ON guacamole_connection_group_permission(entity_id);

--
-- System permissions
--

CREATE TABLE guacamole_system_permission (

  entity_id  INTEGER     NOT NULL,
  permission VARCHAR(16) NOT NULL,

  PRIMARY KEY (entity_id,permission),

  CONSTRAINT guacamole_system_permission_entity
    FOREIGN KEY (entity_id)
    REFERENCES guacamole_entity (entity_id) ON DELETE CASCADE

);

CREATE INDEX ON guacamole_system_permission(entity_id);

--
-- User permissions
--

CREATE TABLE guacamole_user_permission (

  entity_id    INTEGER     NOT NULL,
  affected_user_id INTEGER NOT NULL,
  permission   VARCHAR(16) NOT NULL,

  PRIMARY KEY (entity_id,affected_user_id,permission),

  CONSTRAINT guacamole_user_permission_ibfk_1
    FOREIGN KEY (affected_user_id)
    REFERENCES guacamole_user (user_id) ON DELETE CASCADE,

  CONSTRAINT guacamole_user_permission_entity
    FOREIGN KEY (entity_id)
    REFERENCES guacamole_entity (entity_id) ON DELETE CASCADE

);

CREATE INDEX ON guacamole_user_permission(affected_user_id);
CREATE INDEX ON guacamole_user_permission(entity_id);

--
-- User group permissions
--

CREATE TABLE guacamole_user_group_permission (

  entity_id           INTEGER     NOT NULL,
  affected_user_group_id INTEGER NOT NULL,
  permission          VARCHAR(16) NOT NULL,

  PRIMARY KEY (entity_id,affected_user_group_id,permission),

  CONSTRAINT guacamole_user_group_permission_affected_user_group
    FOREIGN KEY (affected_user_group_id)
    REFERENCES guacamole_user_group (user_group_id) ON DELETE CASCADE,

  CONSTRAINT guacamole_user_group_permission_entity
    FOREIGN KEY (entity_id)
    REFERENCES guacamole_entity (entity_id) ON DELETE CASCADE

);

CREATE INDEX ON guacamole_user_group_permission(affected_user_group_id);
CREATE INDEX ON guacamole_user_group_permission(entity_id);

--
-- User group member users
--

CREATE TABLE guacamole_user_group_member (

  user_group_id    INTEGER     NOT NULL,
  member_entity_id INTEGER     NOT NULL,

  PRIMARY KEY (user_group_id, member_entity_id),

  -- Parent must be a user group
  CONSTRAINT guacamole_user_group_member_parent
    FOREIGN KEY (user_group_id)
    REFERENCES guacamole_user_group (user_group_id) ON DELETE CASCADE,

  -- Member may be either a user or a user group (any entity)
  CONSTRAINT guacamole_user_group_member_entity
    FOREIGN KEY (member_entity_id)
    REFERENCES guacamole_entity (entity_id) ON DELETE CASCADE

);

CREATE INDEX ON guacamole_user_group_member(user_group_id);
CREATE INDEX ON guacamole_user_group_member(member_entity_id);

--
-- Connection history
--

CREATE TABLE guacamole_connection_history (

  history_id           SERIAL           NOT NULL,
  user_id              INTEGER,
  username             VARCHAR(128)     NOT NULL,
  remote_host          VARCHAR(256),
  connection_id        INTEGER,
  connection_name      VARCHAR(128)     NOT NULL,
  sharing_profile_id   INTEGER,
  sharing_profile_name VARCHAR(128),
  start_date           timestamptz      NOT NULL,
  end_date             timestamptz,

  PRIMARY KEY (history_id),

  CONSTRAINT guacamole_connection_history_user
    FOREIGN KEY (user_id)
    REFERENCES guacamole_user (user_id) ON DELETE SET NULL,

  CONSTRAINT guacamole_connection_history_connection
    FOREIGN KEY (connection_id)
    REFERENCES guacamole_connection (connection_id) ON DELETE SET NULL

);

CREATE INDEX ON guacamole_connection_history(user_id);
CREATE INDEX ON guacamole_connection_history(connection_id);
CREATE INDEX ON guacamole_connection_history(start_date);
CREATE INDEX ON guacamole_connection_history(end_date);

--
-- User password history
--

CREATE TABLE guacamole_user_password_history (

  password_history_id SERIAL        NOT NULL,
  user_id             INTEGER       NOT NULL,

  -- Salted password
  password_hash BYTEA        NOT NULL,
  password_salt BYTEA,
  password_date timestamptz  NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY (password_history_id),

  CONSTRAINT guacamole_user_password_history_user
    FOREIGN KEY (user_id)
    REFERENCES guacamole_user (user_id) ON DELETE CASCADE

);

CREATE INDEX ON guacamole_user_password_history(user_id);

-- Create default admin user (username: guacadmin, password: guacadmin)
-- Password hash for 'guacadmin' using default salt
INSERT INTO guacamole_entity (name, type) VALUES ('guacadmin', 'USER');
INSERT INTO guacamole_user (entity_id, password_hash, password_salt)
SELECT 
    entity_id,
    decode('CA458A7D494E3BE824F5E1E175A1556C0F8EEF2C2D7DF3633BEC4A29C4411960', 'hex'),
    decode('FE24ADC5E11E2B25288D1704ABE67A79E342ECC26064CE69C5B3177795A82264', 'hex')
FROM guacamole_entity WHERE name = 'guacadmin' AND type = 'USER';

-- Grant admin all system permissions
INSERT INTO guacamole_system_permission (entity_id, permission)
SELECT entity_id, permission
FROM (
    SELECT entity_id FROM guacamole_entity WHERE name = 'guacadmin' AND type = 'USER'
) AS admin_user
CROSS JOIN (
    VALUES ('CREATE_CONNECTION'),
           ('CREATE_CONNECTION_GROUP'),
           ('CREATE_SHARING_PROFILE'),
           ('CREATE_USER'),
           ('CREATE_USER_GROUP'),
           ('ADMINISTER')
) AS permissions (permission);