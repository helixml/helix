import sqlalchemy as sa
from alembic import op

revision: str = '03'
down_revision = '02'
branch_labels = None
depends_on = None

def upgrade():
    op.alter_column('helix_document_chunk', 'filename',
               existing_type=sa.String(length=255),
               type_=sa.Text,
               existing_nullable=False)


def downgrade():
    op.alter_column('helix_document_chunk', 'filename',
               existing_type=sa.Text,
               type_=sa.String(length=255),
               existing_nullable=False)
