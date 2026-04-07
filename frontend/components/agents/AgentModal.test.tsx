/**
 * @jest-environment jsdom
 */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import AgentModal from './AgentModal';

// Mock the API module
jest.mock('@/lib/api', () => ({
  createAgent: jest.fn().mockResolvedValue({ id: 'new-id', name: 'Test' }),
  updateAgent: jest.fn().mockResolvedValue({}),
  setMemory: jest.fn().mockResolvedValue(undefined),
  deleteMemoryKey: jest.fn().mockResolvedValue(undefined),
  getSchedules: jest.fn().mockResolvedValue([]),
  createSchedule: jest.fn().mockResolvedValue({}),
  deleteSchedule: jest.fn().mockResolvedValue(undefined),
  toggleSchedule: jest.fn().mockResolvedValue(undefined),
}));

test('renders all tabs', () => {
  render(<AgentModal open={true} onClose={() => {}} onSaved={() => {}} />);
  expect(screen.getByText('Basic')).toBeInTheDocument();
  expect(screen.getByText('Memory')).toBeInTheDocument();
  expect(screen.getByText('Guardrails')).toBeInTheDocument();
  expect(screen.getByText('Schedules')).toBeInTheDocument();
});

test('Basic tab form submission calls createAgent', async () => {
  const { createAgent } = require('@/lib/api');
  const onSaved = jest.fn();
  const onClose = jest.fn();

  render(<AgentModal open={true} onClose={onClose} onSaved={onSaved} />);

  // Find inputs by their sibling label text
  const inputs = screen.getAllByRole('textbox');
  // Order: Name, Role, System Prompt (textarea), Model
  fireEvent.change(inputs[0], { target: { value: 'Scout Agent' } });
  fireEvent.change(inputs[1], { target: { value: 'researcher' } });
  fireEvent.change(inputs[2], { target: { value: 'You are a scout.' } });

  fireEvent.click(screen.getByText('Create Agent'));

  await waitFor(() => {
    expect(createAgent).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Scout Agent',
      role: 'researcher',
      system_prompt: 'You are a scout.',
      channels: ['internal'],
    }));
  });
});

test('Memory save calls setMemory with correct payload', async () => {
  const { setMemory } = require('@/lib/api');
  setMemory.mockClear();
  const onSaved = jest.fn();
  const agent = {
    id: 'agent-1', name: 'Bot', role: 'helper', system_prompt: 'hi',
    model: 'claude-sonnet-4-5-20250929', tools: [], channels: ['internal'],
    guardrails: {}, created_at: '', updated_at: '', memory: {},
  };

  render(<AgentModal agent={agent} open={true} onClose={() => {}} onSaved={onSaved} />);

  // Switch to Memory tab — use the tab button
  const memoryTab = screen.getByRole('tab', { name: 'Memory' });
  fireEvent.click(memoryTab);
  // Radix tabs: clicking the trigger makes it active, content panel appears
  fireEvent.keyDown(memoryTab, { key: ' ' });
  fireEvent.keyUp(memoryTab, { key: ' ' });

  // Add a memory entry
  fireEvent.click(screen.getByText('+ Add memory entry'));
  const inputs = screen.getAllByPlaceholderText('Key');
  const values = screen.getAllByPlaceholderText('Value');
  fireEvent.change(inputs[0], { target: { value: 'city' } });
  fireEvent.change(values[0], { target: { value: 'Berlin' } });

  fireEvent.click(screen.getByText('Save Memory'));

  await waitFor(() => {
    expect(setMemory).toHaveBeenCalledWith('agent-1', 'city', 'Berlin');
  });
});
