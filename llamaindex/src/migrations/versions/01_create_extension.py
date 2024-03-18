from alembic import op

revision: str = '01'
down_revision = None
branch_labels = None
depends_on = None

def upgrade() -> None:
  query = """
CREATE EXTENSION IF NOT EXISTS vector;
  """
  op.execute(query)


def downgrade() -> None:
  query = """
DROP EXTENSION IF EXISTS vector;
  """
  op.execute(query)
