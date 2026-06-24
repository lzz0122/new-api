/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Col,
  Empty,
  Input,
  Modal,
  Row,
  Select,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { Activity, History, Play, RefreshCw, Square } from 'lucide-react';
import { API, isAdmin, showError, showSuccess } from '../../helpers';
import { useTranslation } from 'react-i18next';

const { Text, Title } = Typography;

const unwrap = (res) => res?.data?.data ?? res?.data;

const formatTime = (timestamp) => {
  if (!timestamp) return '-';
  return new Date(timestamp * 1000).toLocaleString();
};

const formatQuota = (quota, quotaPerUnit) => {
  const value = Number(quota || 0);
  const unit = Number(quotaPerUnit || 0);
  if (!unit) return value.toLocaleString();
  return (value / unit).toFixed(4);
};

const formatNumber = (value) => Number(value || 0).toLocaleString();

const formatConcurrency = (concurrency) => {
  const used = formatNumber(concurrency?.used);
  const limit = Number(concurrency?.limit || 0);
  if (limit > 0) {
    return `${used} / ${formatNumber(limit)}`;
  }
  return used;
};

const quotaColumns = (quotaPerUnit, t) => [
  {
    title: t('用户'),
    dataIndex: 'username',
    render: (_, record) => record.username || record.email || `#${record.user_id}`,
  },
  {
    title: t('总用量'),
    dataIndex: 'period_quota',
    render: (value) => formatQuota(value, quotaPerUnit),
    sorter: (a, b) => (a.period_quota || 0) - (b.period_quota || 0),
  },
  {
    title: t('请求数'),
    dataIndex: 'period_request_count',
    render: (value) => Number(value || 0).toLocaleString(),
  },
  {
    title: t('令牌数'),
    dataIndex: 'known_tokens',
    render: (value) => Number(value || 0).toLocaleString(),
  },
];

const InfoMetric = ({ label, value }) => (
  <div className='rounded-md border border-gray-200 px-4 py-3'>
    <Text type='secondary' size='small'>
      {label}
    </Text>
    <div className='mt-1 text-lg font-semibold'>{value}</div>
  </div>
);

const SectionDisabled = ({ group, t }) => (
  <Empty
    title={t('该分组暂未开启拼车')}
    description={`${group || '-'} ${t('当前没有进行中的拼车')}`}
  />
);

export default function CarpoolUsage() {
  const { t } = useTranslation();
  const admin = isAdmin();
  const [groups, setGroups] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState('');
  const [status, setStatus] = useState(null);
  const [summary, setSummary] = useState(null);
  const [upstream, setUpstream] = useState(null);
  const [carnival, setCarnival] = useState(null);
  const [history, setHistory] = useState({ months: [], groups: [] });
  const [selectedMonth, setSelectedMonth] = useState('');
  const [selectedSessionId, setSelectedSessionId] = useState(0);
  const [finishVisible, setFinishVisible] = useState(false);
  const [finishCode, setFinishCode] = useState('');
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);

  const selectedSession = useMemo(() => {
    for (const group of history.groups || []) {
      for (const session of group.sessions || []) {
        if (session.id === selectedSessionId) return session;
      }
    }
    return null;
  }, [history, selectedSessionId]);

  const currentOpen = !!summary?.active || !!selectedSessionId;

  const loadGroups = async () => {
    const res = await API.get('/api/carpool-usage/groups');
    const data = unwrap(res) || {};
    const nextGroups = data.groups || [];
    setGroups(nextGroups);
    if (!selectedGroup) {
      setSelectedGroup(
        nextGroups.includes(data.default_group)
          ? data.default_group
          : nextGroups[0] || '',
      );
    }
  };

  const loadGroupData = async (group, month = selectedMonth, sessionId = 0) => {
    if (!group) return;
    setLoading(true);
    try {
      const query = encodeURIComponent(group);
      const summaryUrl = sessionId
        ? `/api/carpool-usage/summary?scope=session&group=${query}&session_id=${sessionId}`
        : `/api/carpool-usage/summary?scope=session&group=${query}`;
      const [statusRes, summaryRes, historyRes, upstreamRes, carnivalRes] =
        await Promise.allSettled([
          API.get(`/api/carpool-usage/status?group=${query}`),
          API.get(summaryUrl),
          API.get(
            `/api/carpool-usage/history?group=${query}${month ? `&month=${month}` : ''}`,
          ),
          API.get(`/api/log/upstream-usage?group=${query}`),
          API.get(`/api/log/carnival?group=${query}`),
        ]);

      if (statusRes.status === 'fulfilled') setStatus(unwrap(statusRes.value));
      if (summaryRes.status === 'fulfilled') setSummary(unwrap(summaryRes.value));
      if (historyRes.status === 'fulfilled') {
        const data = unwrap(historyRes.value) || { months: [], groups: [] };
        setHistory(data);
        if (!month && data.selected_month) setSelectedMonth(data.selected_month);
      }
      setUpstream(
        upstreamRes.status === 'fulfilled' ? unwrap(upstreamRes.value) : null,
      );
      setCarnival(
        carnivalRes.status === 'fulfilled' ? unwrap(carnivalRes.value) : null,
      );
    } catch (error) {
      showError(t('加载拼车统计失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadGroups().catch(() => showError(t('加载分组失败')));
  }, []);

  useEffect(() => {
    if (selectedGroup) {
      setSelectedSessionId(0);
      loadGroupData(selectedGroup, selectedMonth, 0);
    }
  }, [selectedGroup]);

  const refresh = () => loadGroupData(selectedGroup, selectedMonth, selectedSessionId);

  const startCarpool = async () => {
    setActionLoading(true);
    try {
      await API.post(`/api/carpool-usage/start?group=${encodeURIComponent(selectedGroup)}`);
      showSuccess(t('拼车已开启'));
      setSelectedSessionId(0);
      await loadGroupData(selectedGroup, selectedMonth, 0);
    } catch (error) {
      showError(t('开启拼车失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const finishCarpool = async () => {
    setActionLoading(true);
    try {
      const res = await API.post('/api/carpool-usage/finish', {
        group: selectedGroup,
        code: finishCode,
      });
      if (res?.data?.success === false) {
        showError(res.data.message);
        return;
      }
      showSuccess(t('拼车已结束'));
      setFinishVisible(false);
      setFinishCode('');
      setSelectedSessionId(0);
      await loadGroupData(selectedGroup, selectedMonth, 0);
    } catch (error) {
      showError(t('结束拼车失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const loadHistoryMonth = async (month) => {
    setSelectedMonth(month);
    await loadGroupData(selectedGroup, month, selectedSessionId);
  };

  const selectSession = async (session) => {
    setSelectedSessionId(session.id);
    await loadGroupData(selectedGroup, selectedMonth, session.id);
  };

  const quotaPerUnit = summary?.quota_per_unit;
  const totals = summary?.totals || {};
  const activeSession = summary?.session || status?.active || selectedSession;

  return (
    <div className='mt-[60px] px-2'>
      <Spin spinning={loading}>
        <div className='mb-3 flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
          <div>
            <Title heading={3}>{t('拼车统计')}</Title>
            {activeSession ? (
              <Text type='secondary'>
                {formatTime(activeSession.started_at)} -{' '}
                {activeSession.ended_at ? formatTime(activeSession.ended_at) : t('进行中')}
              </Text>
            ) : null}
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            <Select
              value={selectedGroup}
              style={{ width: 180 }}
              placeholder={t('选择分组')}
              onChange={(value) => setSelectedGroup(value)}
            >
              {groups.map((group) => (
                <Select.Option key={group} value={group}>
                  {group}
                </Select.Option>
              ))}
            </Select>
            <Button icon={<RefreshCw size={16} />} onClick={refresh}>
              {t('刷新')}
            </Button>
            {admin && (
              <>
                <Button
                  theme='solid'
                  icon={<Play size={16} />}
                  disabled={!!status?.active || !selectedGroup}
                  loading={actionLoading}
                  onClick={startCarpool}
                >
                  {t('开启拼车')}
                </Button>
                <Button
                  type='danger'
                  icon={<Square size={16} />}
                  disabled={!status?.active}
                  loading={actionLoading}
                  onClick={() => setFinishVisible(true)}
                >
                  {t('结束拼车')}
                </Button>
              </>
            )}
          </div>
        </div>

        {selectedSession ? (
          <Banner
            type='info'
            description={`${t('正在查看历史拼车')}：${formatTime(selectedSession.started_at)} - ${formatTime(selectedSession.ended_at)}`}
            closeIcon={null}
            fullMode={false}
            action={
              <Button size='small' onClick={() => selectSession({ id: 0 })}>
                {t('查看当前拼车')}
              </Button>
            }
            style={{ marginBottom: 12 }}
          />
        ) : null}

        <Row gutter={[12, 12]}>
          <Col xs={24} lg={8}>
            <Card title={t('上游额度')}>
              {!currentOpen ? (
                <SectionDisabled group={selectedGroup} t={t} />
              ) : upstream ? (
                <div className='space-y-3'>
                  <Text strong>{upstream.key_name || upstream.masked_key || '-'}</Text>
                  {upstream.concurrency ? (
                    <InfoMetric
                      label={t('实时并发')}
                      value={formatConcurrency(upstream.concurrency)}
                    />
                  ) : null}
                  {(upstream.rate_limits || []).map((item) => (
                    <InfoMetric
                      key={item.window}
                      label={`${item.window} ${t('剩余额度')}`}
                      value={`${Number(item.remaining || 0).toLocaleString()} / ${Number(item.limit || 0).toLocaleString()}`}
                    />
                  ))}
                </div>
              ) : (
                <Empty title={t('暂无上游额度数据')} />
              )}
            </Card>
          </Col>
          <Col xs={24} lg={8}>
            <Card title={t('拼车额度统计')}>
              {!currentOpen ? (
                <SectionDisabled group={selectedGroup} t={t} />
              ) : (
                <div className='grid grid-cols-1 gap-3'>
                  <InfoMetric
                    label={t('总用量')}
                    value={formatQuota(totals.period_quota, quotaPerUnit)}
                  />
                  <InfoMetric
                    label={t('请求数')}
                    value={Number(totals.period_request_count || 0).toLocaleString()}
                  />
                  <InfoMetric
                    label={t('参与用户')}
                    value={Number(totals.users || 0).toLocaleString()}
                  />
                </div>
              )}
            </Card>
          </Col>
          <Col xs={24} lg={8}>
            <Card title={t('狂欢用量')}>
              {!currentOpen ? (
                <SectionDisabled group={selectedGroup} t={t} />
              ) : (
                <div className='grid grid-cols-1 gap-3'>
                  <InfoMetric
                    label={t('当前狂欢')}
                    value={formatQuota(totals.current_carnival_quota, quotaPerUnit)}
                  />
                  <InfoMetric
                    label={t('本期狂欢')}
                    value={formatQuota(totals.carnival_period_quota, quotaPerUnit)}
                  />
                  <InfoMetric
                    label={t('状态')}
                    value={carnival?.active ? t('进行中') : t('未开启')}
                  />
                </div>
              )}
            </Card>
          </Col>
        </Row>

        <Card
          title={t('各用户用量')}
          style={{ marginTop: 12 }}
          headerExtraContent={
            <Tag color='blue'>{t('已排除狂欢用量')}</Tag>
          }
        >
          {!currentOpen ? (
            <SectionDisabled group={selectedGroup} t={t} />
          ) : (
            <Table
              rowKey='user_id'
              size='small'
              columns={quotaColumns(quotaPerUnit, t)}
              dataSource={summary?.users || []}
              pagination={{ pageSize: 10 }}
            />
          )}
        </Card>

        <Card
          title={
            <span className='inline-flex items-center gap-2'>
              <History size={18} />
              {t('历史拼车')}
            </span>
          }
          style={{ marginTop: 12 }}
          headerExtraContent={
            <Select
              value={selectedMonth}
              style={{ width: 150 }}
              placeholder={t('月份')}
              onChange={loadHistoryMonth}
            >
              {(history.months || []).map((month) => (
                <Select.Option key={month} value={month}>
                  {month}
                </Select.Option>
              ))}
            </Select>
          }
        >
          {(history.groups || []).length === 0 ? (
            <Empty title={t('暂无历史拼车')} />
          ) : (
            <div className='space-y-4'>
              {history.groups.map((group) => (
                <div key={group.group}>
                  <div className='mb-2 flex items-center gap-2'>
                    <Activity size={16} />
                    <Text strong>{group.group}</Text>
                    <Tag>
                      {t('总用量')} {formatQuota(group.total?.period_quota, quotaPerUnit)}
                    </Tag>
                  </div>
                  <Table
                    rowKey='id'
                    size='small'
                    pagination={false}
                    dataSource={group.sessions || []}
                    columns={[
                      {
                        title: t('开始时间'),
                        dataIndex: 'started_at',
                        render: formatTime,
                      },
                      {
                        title: t('结束时间'),
                        dataIndex: 'ended_at',
                        render: (value) => (value ? formatTime(value) : t('进行中')),
                      },
                      {
                        title: t('总用量'),
                        dataIndex: 'total_quota',
                        render: (value) => formatQuota(value, quotaPerUnit),
                      },
                      {
                        title: t('操作'),
                        render: (_, record) => (
                          <Button size='small' onClick={() => selectSession(record)}>
                            {t('查看')}
                          </Button>
                        ),
                      },
                    ]}
                  />
                </div>
              ))}
            </div>
          )}
        </Card>
      </Spin>

      <Modal
        title={t('结束拼车')}
        visible={finishVisible}
        onCancel={() => setFinishVisible(false)}
        onOk={finishCarpool}
        confirmLoading={actionLoading}
      >
        <Input
          type='password'
          value={finishCode}
          placeholder={t('输入拼车结束 2FA 密码')}
          onChange={setFinishCode}
        />
      </Modal>
    </div>
  );
}
